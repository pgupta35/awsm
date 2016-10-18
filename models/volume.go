package models

import "time"

type Volume struct {
	Name             string    `json:"name" awsmTable:"Name"`
	Class            string    `json:"class" awsmTable:"Class"`
	VolumeID         string    `json:"volumeID" awsmTable:"Volume ID"`
	Size             int       `json:"size"`
	SizeHuman        string    `json:"sizeHuman" awsmTable:"Size"`
	State            string    `json:"state" awsmTable:"State"`
	Iops             string    `json:"iops" awsmTable:"IOPS"`
	InstanceID       string    `json:"instanceID" awsmTable:"Instance ID"`
	Attachment       string    `json:"attachment" awsmTable:"Attachment"`
	CreationTime     time.Time `json:"creationTime"`
	CreatedHuman     string    `json:"createdHuman" awsmTable:"Created"`
	VolumeType       string    `json:"volumeType" awsmTable:"Volume Type"`
	SnapshoID        string    `json:"snapshotID"`
	DeleteOnTerm     bool      `json:"deleteOnTerm" awsmTable:"Delete On Term."`
	AvailabilityZone string    `json:"availabilityZone" awsmTable:"Availability Zone"`
	Region           string    `json:"region" awsmTable:"Region"`
}
