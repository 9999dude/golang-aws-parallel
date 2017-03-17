package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"gopkg.in/yaml.v2"
)

//YAMLConfig .. Structure of YAMLConfig type to read yaml data.
type YAMLConfig struct {
	ExcludeAMI           []string `yaml:"exclude_ami"`
	AWSRegion            string   `yaml:"aws_region"`
	AWSCredentialFile    string   `yaml:"aws_credential_file"`
	AWSCredentialProfile string   `yaml:"aws_credential_profile"`
	NoOfExecuter         int      `yaml:"no_of_executer"`
	Duration             int      `yaml:"duration"`
	AWSAccountID         string   `yaml:"aws_account_id"`
	Dryrun               bool     `yaml:"dryrun"`
	LogLocation          string   `yaml:"log_location"`
}

var (
	Log        *log.Logger
	yamlconfig YAMLConfig
)

//WorkerPool .. This function will create worker pool of go routines. It takes data from jobs channel and
//assignes it to a goroutine. Once the task is done by the goroutine, the result is written to results
//channel. Number of goroutine in the worker pool is controlled by the calling function, i.e. main() here.
func WorkerPool(id int, jobs <-chan string, results chan<- string, svc *ec2.EC2, DryRun bool) {
	for j := range jobs {
		Log.Println("Goroutine id.", id, "deregistering ami id.", j)
		DeregisterAmi(j, svc, DryRun)
		results <- j
	}
}

//DeregisterAmi .. Code to deregister ami.
func DeregisterAmi(amiID string, svc *ec2.EC2, DryRun bool) {
	params := &ec2.DeregisterImageInput{
		ImageId: aws.String(amiID),
		DryRun:  aws.Bool(DryRun),
	}
	_, err := svc.DeregisterImage(params)

	if err != nil {
		Log.Println(err.Error())
		return
	}
}

//AmiCheck .. Returns true if the ami id is in the exclusion list (in the yaml).
func AmiCheck(AmiID string, yamlconfig YAMLConfig) bool {
	for _, b := range yamlconfig.ExcludeAMI {
		if b == AmiID {
			return true
		}
	}
	return false
}

func main() {
	var config = flag.String("config", "config.yaml", "Config file path. Please copy the config.yaml to the appropriate path.")
	flag.Parse()

	//Parsing yaml data
	source, FileErr := ioutil.ReadFile(*config)
	if FileErr != nil {
		panic(FileErr)
	}
	FileErr = yaml.Unmarshal(source, &yamlconfig)
	if FileErr != nil {
		panic(FileErr)
	}
	DryRun := yamlconfig.Dryrun
	AWSRegion := yamlconfig.AWSRegion
	AWSCredentialFile := yamlconfig.AWSCredentialFile
	AWSCredentialProfile := yamlconfig.AWSCredentialProfile
	NoOfExecuter := yamlconfig.NoOfExecuter
	Duration := yamlconfig.Duration
	AWSAccountID := yamlconfig.AWSAccountID
	LogLocation := yamlconfig.LogLocation

	// Setting up log path.
	file, FileErr := os.Create(LogLocation)
	if FileErr != nil {
		panic(FileErr)
	}
	Log = log.New(file, "", log.LstdFlags|log.Lshortfile)

	delta := int64(Duration)
	t := time.Now().Unix()

	//Load aws iam credentials
	creds := credentials.NewSharedCredentials(AWSCredentialFile, AWSCredentialProfile)
	_, err := creds.Get()
	if err != nil {
		panic(err)
	}

	// Create an EC2 service object
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(AWSRegion),
		Credentials: creds,
	})

	paramsImage := &ec2.DescribeImagesInput{
		Owners: []*string{
			aws.String(AWSAccountID),
		},
	}
	resp, err := svc.DescribeImages(paramsImage)
	if err != nil {
		panic(err)
	}

	//Creating worker job pool. Number of goroutines to run is configured by -executer command line parameter.
	jobs := make(chan string)
	results := make(chan string)
	for w := 1; w <= NoOfExecuter; w++ {
		go WorkerPool(w, jobs, results, svc, DryRun)
	}

	for _, image := range resp.Images {
		// Check if ami is listed in the exclustion list.
		status := AmiCheck(*image.ImageId, yamlconfig)
		if status {
			continue
		}

		//Next three line of code is required as bloody Amazon is returning *image.CreationDate
		//as string instead of time.Time object
		Date := strings.Split(strings.Split(*image.CreationDate, ".")[0], "T")[0] + " " + strings.Split(strings.Split(*image.CreationDate, ".")[0], "T")[1]
		datetime, _ := time.Parse("2006-01-02 15:04:05", Date)
		AMICreationDate := datetime.Unix()
		if t-AMICreationDate > delta {
			if image.Name == nil {
				Log.Println("Trying to deregister ami ", *image.CreationDate, *image.ImageId, "No name specified")
				go func(image *ec2.Image) {
					jobs <- *image.ImageId
				}(image)
				<-results
			} else {
				Log.Println("Trying to deregister ami ", *image.CreationDate, *image.ImageId, *image.Name)
				go func(image *ec2.Image) {
					jobs <- *image.ImageId
				}(image)
				<-results
			}
		}
	}
	close(jobs)
}
