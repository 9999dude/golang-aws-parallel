package main

import (
	"io/ioutil"
	"log"
	"os"
	"time"

	"flag"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"gopkg.in/yaml.v2"
)

//YAMLConfig .. Structure of YAMLConfig type to read yaml data.
type YAMLConfig struct {
	Aws_region             string
	Aws_credential_file    string
	Aws_credential_profile string
	No_of_executer         int
	Duration               int
	Aws_account_id         string
	Dryrun                 bool
	Log_location           string
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
		Log.Println("Goroutine id", id, "removing snapshot id.", j)
		RemoveSnapshot(j, svc, DryRun)
		results <- j
	}
}

//RemoveSnapshot .. Code to remove snapshot
func RemoveSnapshot(snapshotID string, svc *ec2.EC2, DryRun bool) {
	params := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(snapshotID),
		DryRun:     aws.Bool(DryRun),
	}

	resp, err := svc.DeleteSnapshot(params)
	if err != nil {
		Log.Println(err.Error())
		return
	}

	Log.Println(resp)
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
	AWSRegion := yamlconfig.Aws_region
	AWSCredentialFile := yamlconfig.Aws_credential_file
	AWSCredentialProfile := yamlconfig.Aws_credential_profile
	NoOfExecuter := yamlconfig.No_of_executer
	Duration := yamlconfig.Duration
	AWSAccountID := yamlconfig.Aws_account_id
	LogLocation := yamlconfig.Log_location

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

	params := &ec2.DescribeSnapshotsInput{
		OwnerIds: []*string{
			aws.String(AWSAccountID),
		},
	}

	resp, err := svc.DescribeSnapshots(params)
	if err != nil {
		panic(err)
	}

	//Creating worker job pool. Number of goroutines to run is configured by -executer command line parameter.
	jobs := make(chan string)
	results := make(chan string)
	for w := 1; w <= NoOfExecuter; w++ {
		go WorkerPool(w, jobs, results, svc, DryRun)
	}

	for _, snapshot := range resp.Snapshots {
		if t-snapshot.StartTime.Unix() > delta && *snapshot.State == "completed" {
			Log.Println("Trying to remove snapshot ", *snapshot.SnapshotId, *snapshot.State, *snapshot.StartTime)
			go func(snapshot *ec2.Snapshot) {
				jobs <- *snapshot.SnapshotId
			}(snapshot)
			<-results
		}
	}
}
