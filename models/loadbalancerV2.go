package models

import "time"

type LoadBalancerV2 struct {
	Name                  string    `json:"name"`
	DNSName               string    `json:"dnsName"`
	Type                  string    `json:"type"`
	State                 string    `json:"state"`
	Region                string    `json:"region"`
	AvailabilityZones     string    `json:"availabilityZone"`
	CreatedTime           time.Time `json:"createdTime"`
	CreatedHuman          string    `json:"createdHuman"`
	SecurityGroups        string    `json:"securityGroups"`
	Scheme                string    `json:"scheme"`
	CanonicalHostedZoneID string    `json:"canonicalHostedZoneID"`
	LoadBalancerArn       string    `json:"loadBalancerArn"`
	VpcID                 string    `json:"vpcID"`
}
