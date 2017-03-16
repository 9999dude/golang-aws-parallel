### Introduction

The aim of this project is to let users delete multiple AWS volume snapshots in parallel.


### Dependency

* Go 1.6+. Please follow the [link](https://golang.org/doc/install) to set up Go environment.
* Install following dependencies.
  ```bash
  go get -u github.com/aws/aws-sdk-go
  go get -u gopkg.in/yaml.v2
  ```

### Setup and execution
* You can build the code and execute in the following fashion.
  ```bash
  $ cd delete-snapshots-parallel
  $ go build
  $ ./delete-snapshots-parallel -config config.yaml
  ```

### Configuration
* I am shipping config.yaml with this project. It contains some default value. Please adjust accordingly.

#### Note:
Please make sure to do a dry run first. Setting "dryrun" flag to true in config.yaml will perform dry run. Once you are satisfied with the result. Make it false and re-execute.
