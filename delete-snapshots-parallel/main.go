package main

import (
	"fmt"
	"os"
	"time"

	"flag"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

//WorkerPool .. This function will create worker pool of go routines. It takes data from jobs channel and
//assignes it to a goroutine. Once the task is done by the goroutine, the result is written to results
//channel. Number of goroutine in the worker pool is controlled by the calling function, i.e. main() here.
func WorkerPool(id int, jobs <-chan string, results chan<- string, svc *ec2.EC2) {
	for j := range jobs {
		fmt.Println("Goroutine id", id, "removing snapshot id.", j)
		RemoveSnapshot(j, svc)
		results <- j
	}
}

//RemoveSnapshot .. Code to remove snapshot
func RemoveSnapshot(snapshotID string, svc *ec2.EC2) {
	params := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(snapshotID),
		//DryRun:     aws.Bool(true),
	}

	resp, err := svc.DeleteSnapshot(params)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Println(resp)
}

func main() {
	const (
		awsRegion            = "ap-southeast-1"
		awsCredentialFile    = "/root/.aws/config"
		awsCredentialProfile = "default"
	)

	var duration = flag.Int("duration", 604800, "In seconds. AMI older than this duration will be terminated")
	var executer = flag.Int("executer", 4, "Number of goroutines to run in a worker pool. By default, running 4 goroutines")
	flag.Parse()

	delta := int64(*duration)
	t := time.Now().Unix()

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

	params := &ec2.DescribeSnapshotsInput{
		//DryRun: aws.Bool(true),
		OwnerIds: []*string{
			aws.String("470661122947"),
		},
	}

	resp, err := svc.DescribeSnapshots(params)
	if err != nil {
		panic(err)
	}

	//Creating worker job pool. Number of goroutines to run is configured by -executer command line parameter.
	jobs := make(chan string)
	results := make(chan string)
	for w := 1; w <= *executer; w++ {
		go WorkerPool(w, jobs, results, svc)
	}

	for _, snapshot := range resp.Snapshots {
		if t-snapshot.StartTime.Unix() > delta && *snapshot.State == "completed" {
			fmt.Println("Trying to remove snapshot ", *snapshot.SnapshotId, *snapshot.State, *snapshot.StartTime)
			//RemoveSnapshot(*snapshot.SnapshotId, svc)
			go func(snapshot *ec2.Snapshot) {
				jobs <- *snapshot.SnapshotId
			}(snapshot)
			<-results
		}
	}
}
