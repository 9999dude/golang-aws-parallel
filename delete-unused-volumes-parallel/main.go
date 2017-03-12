package main

import (
	"flag"
	"fmt"
	"os"
	"time"

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
		DeleteUnusedVolumes(j, svc)
		results <- j
	}
}

//DeleteUnusedVolumes .. Code to remove ebs volumes
func DeleteUnusedVolumes(volumeID string, svc *ec2.EC2) {

	params := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volumeID),
		//DryRun:   aws.Bool(true),
	}
	_, err := svc.DeleteVolume(params)
	if err != nil {
		fmt.Println(err)
	}

}

func main() {
	const (
		awsRegion            = "ap-southeast-1"
		awsCredentialFile    = "/root/.aws/config"
		awsCredentialProfile = "default"
	)

	var duration = flag.Int("duration", 604800, "In seconds. Available volumes older than this duration will be terminated")
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

	resp, err := svc.DescribeVolumes(nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	//Creating worker job pool. Number of goroutines to run is configured by -executer command line parameter.
	jobs := make(chan string)
	results := make(chan string)
	for w := 1; w <= *executer; w++ {
		go WorkerPool(w, jobs, results, svc)
	}

	for _, volume := range resp.Volumes {
		if t-volume.CreateTime.Unix() > delta && *volume.State == "available" {
			fmt.Println("Trying to remove volume ", *volume.CreateTime, *volume.VolumeId, *volume.AvailabilityZone, *volume.State)
			go func(volume *ec2.Volume) {
				jobs <- *volume.VolumeId
			}(volume)
			<-results
		}
	}

}
