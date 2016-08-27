package config

import (
	"strconv"

	"github.com/aws/aws-sdk-go/service/simpledb"
)

type InstanceClassConfigs map[string]InstanceClassConfig

type InstanceClassConfig struct {
	InstanceType     string
	SecurityGroups   []string
	EBSVolumes       []string
	Vpc              string
	Subnet           string
	PublicIpAddress  bool
	AMI              string
	KeyName          string
	EbsOptimized     bool
	Monitoring       bool
	ShutdownBehavior string
	IAMUser          string
	UserData         string
}

func DefaultInstanceClasses() InstanceClassConfigs {
	defaultInstances := make(InstanceClassConfigs)

	defaultInstances["base"] = InstanceClassConfig{
		InstanceType:     "t1.micro",
		SecurityGroups:   []string{"dev"},
		EBSVolumes:       []string{},
		Vpc:              "awsm",
		Subnet:           "private",
		PublicIpAddress:  false,
		AMI:              "base",
		KeyName:          "awsm",
		ShutdownBehavior: "terminate",
	}

	defaultInstances["dev"] = InstanceClassConfig{
		InstanceType:     "r3.large",
		SecurityGroups:   []string{"all", "dev"},
		EBSVolumes:       []string{"git-standard", "mysql-data-standard"}, // TODO
		Vpc:              "awsm",
		Subnet:           "private",
		PublicIpAddress:  false,
		AMI:              "hvm-base",
		KeyName:          "awsm",
		ShutdownBehavior: "terminate",
		UserData:         "#!/bin/bash \n echo wemadeit > ~/didwemakeit",
	}

	defaultInstances["prod"] = InstanceClassConfig{
		InstanceType:     "r3.large",
		SecurityGroups:   []string{"dev"},
		EBSVolumes:       []string{},
		Vpc:              "awsm",
		Subnet:           "private",
		PublicIpAddress:  false,
		AMI:              "hvm-base",
		KeyName:          "awsm",
		ShutdownBehavior: "terminate",
	}

	return defaultInstances
}

func (c *InstanceClassConfig) LoadConfig(class string) error {

	data, err := GetClassConfig("instances", class)
	if err != nil {
		return err
	}

	c.Marshal(data.Attributes)

	return nil
}

func (c *InstanceClassConfig) Marshal(attributes []*simpledb.Attribute) {

	for _, attribute := range attributes {

		val := *attribute.Value

		switch *attribute.Name {

		case "InstanceType":
			c.InstanceType = val

		case "SecurityGroups":
			c.SecurityGroups = append(c.SecurityGroups, val)

		case "EBSVolumes":
			c.EBSVolumes = append(c.EBSVolumes, val)

		case "Subnet":
			c.Subnet = val

		case "Vpc":
			c.Vpc = val

		case "PublicIpAddress":
			c.PublicIpAddress, _ = strconv.ParseBool(val)

		case "AMI":
			c.AMI = val

		case "KeyName":
			c.KeyName = val

		case "EbsOptimized":
			c.EbsOptimized, _ = strconv.ParseBool(val)

		case "Monitoring":
			c.Monitoring, _ = strconv.ParseBool(val)

		case "ShutdownBehavior":
			c.ShutdownBehavior = val

		case "UserData":
			c.UserData = val

		case "IAMUser":
			c.IAMUser = val

		}
	}
}
