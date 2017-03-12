package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

//Config .. Structure of Config type to read yaml data.
type Config struct {
	Amiid []string
}

//WorkerPool .. This function will create worker pool of go routines. It takes data from jobs channel and
//assignes it to a goroutine. Once the task is done by the goroutine, the result is written to results
//channel. Number of goroutine in the worker pool is controlled by the calling function, i.e. main() here.
func WorkerPool(id int, jobs <-chan string, results chan<- string, svc *ec2.EC2) {
	for j := range jobs {
		fmt.Println("Goroutine id.", id, "deregistering ami id.", j)
		DeregisterAmi(j, svc)
		results <- j
	}
}

//DeregisterAmi .. Code to deregister an ami.
func DeregisterAmi(amiID string, svc *ec2.EC2) {
	params := &ec2.DeregisterImageInput{
		ImageId: aws.String(amiID),
		//DryRun:  aws.Bool(true),
	}
	_, err := svc.DeregisterImage(params)

	if err != nil {
		fmt.Println(err.Error())
		return
	}
}

//AmiCheck .. Returns true if the ami id is in the exclusion list (in the yaml).
func AmiCheck(AmiID string, config Config) bool {
	for _, b := range config.Amiid {
		if b == AmiID {
			return true
		}
	}
	return false
}

func main() {
	const (
		awsRegion            = "ap-southeast-1"
		awsCredentialFile    = "/root/.aws/config"
		awsCredentialProfile = "default"
	)

	var duration = flag.Int("duration", 604800, "In seconds. AMI older than this duration will be terminated")
	var exclude = flag.String("exclude", "exclude.yaml", "Ami to exclude should be put in this file")
	var executer = flag.Int("executer", 4, "Number of goroutines to run in a worker pool. By default, running 4 goroutines")

	flag.Parse()

	delta := int64(*duration)
	t := time.Now().Unix()

	//Parsing yaml data
	var config Config
	fmt.Println(*exclude)
	source, FileErr := ioutil.ReadFile(*exclude)
	if FileErr != nil {
		panic(FileErr)
	}
	FileErr = yaml.Unmarshal(source, &config)
	if FileErr != nil {
		panic(FileErr)
	}

	//Load aws iam credentials
	creds := credentials.NewSharedCredentials(awsCredentialFile, awsCredentialProfile)
	_, err := creds.Get()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create an EC2 service object
	svc := ec2.New(session.New(), &aws.Config{
		Region:      aws.String(awsRegion),
		Credentials: creds,
	})

	paramsImage := &ec2.DescribeImagesInput{
		Owners: []*string{
			aws.String("xxxxxxxxxxxx"),
		},
	}
	resp, err := svc.DescribeImages(paramsImage)
	if err != nil {
		panic(err)
	}

	//Creating worker job pool. Number of goroutines to run is configured by -executer command line parameter.
	jobs := make(chan string)
	results := make(chan string)
	for w := 1; w <= *executer; w++ {
		go WorkerPool(w, jobs, results, svc)
	}

	for _, image := range resp.Images {
		status := AmiCheck(*image.ImageId, config)
		if status {
			continue
		}

		//Next three line of code is required as bloody amazon is returning *image.CreationDate
		//as string instead of time.Time object
		Date := strings.Split(strings.Split(*image.CreationDate, ".")[0], "T")[0] + " " + strings.Split(strings.Split(*image.CreationDate, ".")[0], "T")[1]
		datetime, _ := time.Parse("2006-01-02 15:04:05", Date)
		AMICreationDate := datetime.Unix()
		if t-AMICreationDate > delta {
			if image.Name == nil {
				fmt.Println("Trying to deregister ami ", *image.CreationDate, *image.ImageId, "No name specified")
				go func(image *ec2.Image) {
					jobs <- *image.ImageId
				}(image)
				<-results
			} else {
				fmt.Println("Trying to deregister ami ", *image.CreationDate, *image.ImageId, *image.Name)
				go func(image *ec2.Image) {
					jobs <- *image.ImageId
				}(image)
				<-results
			}
		}
	}
	close(jobs)
}
