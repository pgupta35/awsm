package aws

import (
	"errors"
	"strings"

	"github.com/murdinc/awsm/models"
)

/*
	Went the route of parsing ARNs because some AWS resources reference
	each other by ARN, like Alarms and Scaling Policies. Hitting the API
	to make these human friendly before Marshalling would cause an infinite-loop. :-(
*/

type ARN models.ARN

func ParseArn(arnStr string) (*ARN, error) {
	split := strings.SplitN(arnStr, ":", -1)

	if len(split) < 6 {
		return &ARN{}, errors.New("Error parsing ARN string!")
	}

	arn := new(ARN)
	arn.Arn = split[0]
	arn.Partition = split[1]
	arn.Service = split[2]
	arn.Region = split[3]
	arn.AccountID = split[4]

	// TODO finish detection of other types of ARNs
	switch arn.Service {
	case "autoscaling":
		arn.ResourceType = split[5]
		switch arn.ResourceType {
		case "scalingPolicy":
			arn.PolicyID = split[6]
			arn.AutoScalingGroupName = strings.TrimLeft(split[7], "autoScalingGroupName/")
			arn.PolicyName = strings.TrimLeft(split[8], "policyName/")
		case "autoScalingGroup":
			arn.GroupID = split[6]
			arn.AutoScalingGroupName = split[7]
		}

	default:
		if len(split) == 6 {
			arn.Resource = split[5]
		} else {
			arn.ResourceType = split[5]
			arn.Resource = split[6]
		}
	}

	return arn, nil
}
