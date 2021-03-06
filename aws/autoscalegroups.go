package aws

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/murdinc/awsm/aws/regions"
	"github.com/murdinc/awsm/config"
	"github.com/murdinc/awsm/models"
	"github.com/murdinc/cli"
	"github.com/murdinc/terminal"
	"github.com/olekukonko/tablewriter"
)

// AutoScaleGroups represents a slice of AutoScale Groups
type AutoScaleGroups []AutoScaleGroup

// AutoScaleGroup represents a single AutoScale Group
type AutoScaleGroup models.AutoScaleGroup

// AutoScaleGroups represents a slice of AutoScale Groups
type ScalingActivities []ScalingActivity

// AutoScaleGroup represents a single AutoScale Group
type ScalingActivity models.ScalingActivity

// GetAutoScaleGroups returns a slice of AutoScale Groups based on the given search term
func GetAutoScaleGroups(search string) (*AutoScaleGroups, []error) {
	var wg sync.WaitGroup
	var errs []error

	asgList := new(AutoScaleGroups)
	regions := GetRegionListWithoutIgnored()

	for _, region := range regions {
		wg.Add(1)

		go func(region *ec2.Region) {
			defer wg.Done()
			err := GetRegionAutoScaleGroups(*region.RegionName, asgList, search)
			if err != nil {
				terminal.ShowErrorMessage(fmt.Sprintf("Error gathering autoscale group list for region [%s]", *region.RegionName), err.Error())
				errs = append(errs, err)
			}
		}(region)
	}
	wg.Wait()

	return asgList, errs
}

// GetRegionAutoScaleGroups returns a list of AutoScale Groups for a given region into the provided AutoScaleGroups slice
func GetRegionAutoScaleGroups(region string, asgList *AutoScaleGroups, search string) error {

	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
	svc := autoscaling.New(sess)

	result, err := svc.DescribeAutoScalingGroups(&autoscaling.DescribeAutoScalingGroupsInput{})
	if err != nil {
		return err
	}

	subList := new(Subnets)
	GetRegionSubnets(region, subList, "")

	asg := make(AutoScaleGroups, len(result.AutoScalingGroups))
	for i, autoscalegroup := range result.AutoScalingGroups {
		asg[i].Marshal(autoscalegroup, region, subList)
	}

	if search != "" {
		term := regexp.MustCompile(search)
	Loop:
		for i, g := range asg {
			rAsg := reflect.ValueOf(g)

			for k := 0; k < rAsg.NumField(); k++ {
				sVal := rAsg.Field(k).String()

				if term.MatchString(sVal) {
					*asgList = append(*asgList, asg[i])
					continue Loop
				}
			}
		}
	} else {
		*asgList = append(*asgList, asg[:]...)
	}

	return nil
}

// GetScalingActivities
func GetScalingActivities(search string, latest bool) (ScalingActivities, error) {

	asgList, errs := GetAutoScaleGroups(search)
	if errs != nil {
		return nil, errors.New("Error while retrieving the list of AutoScale Groups!")
	}

	if len(*asgList) > 0 {
		// Print the table
		asgList.PrintTable()
	} else {
		return nil, errors.New("No AutoScaling Groups found, Aborting!")
	}

	activities, err := getScalingActivities(asgList, latest)
	if err != nil {
		return nil, err
	}

	terminal.Information("Done!")

	return activities, nil
}

// private function with no terminal prompts
func getScalingActivities(asgList *AutoScaleGroups, latest bool) (activities ScalingActivities, err error) {

	for _, asg := range *asgList {

		terminal.Delta("Gathering Scaling Activities for AutoScale Group [" + asg.Name + "] in [" + asg.Region + "]")

		sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(asg.Region)}))
		svc := autoscaling.New(sess)

		// Gather the activities
		params := &autoscaling.DescribeScalingActivitiesInput{
			AutoScalingGroupName: aws.String(asg.Name),
		}

		if latest {
			params.SetMaxRecords(1)
		}

		result, err := svc.DescribeScalingActivities(params)
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				return activities, errors.New(awsErr.Message())
			}
			return activities, err
		}

		asgActivities := make(ScalingActivities, len(result.Activities))
		for i, activity := range result.Activities {
			asgActivities[i].Marshal(activity, &asg)
		}

		activities = append(activities, asgActivities...)

	}

	return activities, nil
}

// Marshal parses the response from the aws sdk into an awsm AutoScale Group
func (a *AutoScaleGroup) Marshal(autoscalegroup *autoscaling.Group, region string, subList *Subnets) {
	a.Name = aws.StringValue(autoscalegroup.AutoScalingGroupName)
	a.Class = GetTagValue("Class", autoscalegroup.Tags)
	a.HealthCheckType = aws.StringValue(autoscalegroup.HealthCheckType)
	a.HealthCheckGracePeriod = int(aws.Int64Value(autoscalegroup.HealthCheckGracePeriod))
	a.LaunchConfig = aws.StringValue(autoscalegroup.LaunchConfigurationName)
	a.LoadBalancers = aws.StringValueSlice(autoscalegroup.LoadBalancerNames)
	a.InstanceCount = len(autoscalegroup.Instances)
	a.DesiredCapacity = int(aws.Int64Value(autoscalegroup.DesiredCapacity))
	a.MinSize = int(aws.Int64Value(autoscalegroup.MinSize))
	a.MaxSize = int(aws.Int64Value(autoscalegroup.MaxSize))
	a.DefaultCooldown = int(aws.Int64Value(autoscalegroup.DefaultCooldown))
	a.AvailabilityZones = aws.StringValueSlice(autoscalegroup.AvailabilityZones)
	a.SubnetID = aws.StringValue(autoscalegroup.VPCZoneIdentifier)
	a.SubnetName = subList.GetSubnetName(a.SubnetID)
	a.VpcID = subList.GetVpcIDBySubnetID(a.SubnetID)
	a.VpcName = subList.GetVpcNameBySubnetID(a.SubnetID)
	a.Region = region
}

// Marshal parses the response from the aws sdk into an awsm AutoScale Group
func (a *ScalingActivity) Marshal(activity *autoscaling.Activity, asg *AutoScaleGroup) {

	a.ActivityId = aws.StringValue(activity.ActivityId)
	a.AutoScalingGroupName = aws.StringValue(activity.AutoScalingGroupName)
	a.Cause = aws.StringValue(activity.Cause)
	a.Description = aws.StringValue(activity.Description)
	a.Details = aws.StringValue(activity.Details)
	a.EndTime = aws.TimeValue(activity.EndTime)
	a.Progress = int(aws.Int64Value(activity.Progress))
	a.StartTime = aws.TimeValue(activity.StartTime)
	a.StatusCode = aws.StringValue(activity.StatusCode)
	a.Region = asg.Region
}

// LockedLaunchConfigurations returns a map of Launch Configurations that are locked (currently being used in an AutoScale Group)
func (a *AutoScaleGroups) LockedLaunchConfigurations() map[string]bool {

	names := make(map[string]bool, len(*a))
	for _, asg := range *a {
		names[asg.LaunchConfig] = true
	}
	return names
}

// CreateAutoScaleGroups creates a new AutoScale Group of the given class
func CreateAutoScaleGroups(class string, dryRun bool) (err error) {

	// --dry-run flag
	if dryRun {
		terminal.Information("--dry-run flag is set, not making any actual changes!")
	}

	// Verify the asg config class input
	cfg, err := config.LoadAutoscalingGroupClass(class)
	if err != nil {
		return err
	}
	terminal.Information("Found Autoscaling group class configuration for [" + class + "]")

	// Verify the launchconfig class input
	launchConfigurationCfg, err := config.LoadLaunchConfigurationClass(cfg.LaunchConfigurationClass)
	if err != nil {
		return err
	}
	terminal.Information("Found Launch Configuration class configuration for [" + cfg.LaunchConfigurationClass + "]")

	// Get the AZs
	azs, errs := regions.GetAZs()
	if errs != nil {
		return errors.New("Error gathering region list")
	}

	asgList := new(AutoScaleGroups)

	for region, regionAZs := range azs.GetRegionMap(cfg.AvailabilityZones) {

		*asgList = append(*asgList, AutoScaleGroup{
			Name:   class,
			Region: region,
		})

		// Verify that the latest Launch Configuration is available in this region
		lcName := GetLaunchConfigurationName(region, cfg.LaunchConfigurationClass, launchConfigurationCfg.Version)
		if lcName == "" {
			return fmt.Errorf("Launch Configuration [%s] version [%d] is not available in [%s]!", cfg.LaunchConfigurationClass, launchConfigurationCfg.Version, region)
		}
		terminal.Information(fmt.Sprintf("Found latest Launch Configuration [%s] version [%d] in [%s]", cfg.LaunchConfigurationClass, launchConfigurationCfg.Version, region))

		sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
		svc := autoscaling.New(sess)

		params := &autoscaling.CreateAutoScalingGroupInput{
			AutoScalingGroupName:    aws.String(class),
			MaxSize:                 aws.Int64(int64(cfg.MaxSize)),
			MinSize:                 aws.Int64(int64(cfg.MinSize)),
			DefaultCooldown:         aws.Int64(int64(cfg.DefaultCooldown)),
			DesiredCapacity:         aws.Int64(int64(cfg.DesiredCapacity)),
			HealthCheckGracePeriod:  aws.Int64(int64(cfg.HealthCheckGracePeriod)),
			HealthCheckType:         aws.String(cfg.HealthCheckType),
			LaunchConfigurationName: aws.String(lcName),

			// TODO ?
			// InstanceId:                       aws.String("XmlStringMaxLen19"),
			// NewInstancesProtectedFromScaleIn: aws.Bool(true),
			// PlacementGroup:                   aws.String("XmlStringMaxLen255"),
			Tags: []*autoscaling.Tag{
				{
					// Name
					Key:               aws.String("Name"),
					PropagateAtLaunch: aws.Bool(true),
					ResourceId:        aws.String(class),
					ResourceType:      aws.String("auto-scaling-group"),
					Value:             aws.String(lcName),
				},
				{
					// Class
					Key:               aws.String("Class"),
					PropagateAtLaunch: aws.Bool(true),
					ResourceId:        aws.String(class),
					ResourceType:      aws.String("auto-scaling-group"),
					Value:             aws.String(cfg.LaunchConfigurationClass),
				},
			},
		}

		subList := new(Subnets)
		var vpcZones []string

		if cfg.SubnetClass != "" {
			err := GetRegionSubnets(region, subList, "")
			if err != nil {
				return err
			}
		}

		// Set the AZs
		for _, az := range regionAZs {
			if !azs.ValidAZ(az) {
				return cli.NewExitError("Availability Zone ["+az+"] is Invalid!", 1)
			}
			terminal.Information("Found Availability Zone [" + az + "]!")

			params.AvailabilityZones = append(params.AvailabilityZones, aws.String(az))

			for _, sub := range *subList {
				if sub.Class == cfg.SubnetClass && sub.AvailabilityZone == az {
					vpcZones = append(vpcZones, sub.SubnetID)
				}
			}

		}

		// Set the VPCZoneIdentifier (SubnetIds seperated by comma)
		params.VPCZoneIdentifier = aws.String(strings.Join(vpcZones, ", "))

		// Set the Load Balancers
		for _, elb := range cfg.LoadBalancerNames {
			params.LoadBalancerNames = append(params.LoadBalancerNames, aws.String(elb))
		}

		// Set the Termination Policies
		for _, terminationPolicy := range cfg.TerminationPolicies {
			params.TerminationPolicies = append(params.TerminationPolicies, aws.String(terminationPolicy))
		}

		// Create it!
		if !dryRun {
			_, err := svc.CreateAutoScalingGroup(params)
			if err != nil {
				if awsErr, ok := err.(awserr.Error); ok {
					return errors.New(awsErr.Message())
				}
				return err
			}

			terminal.Delta("Created AutoScaling Group [" + aws.StringValue(params.AutoScalingGroupName) + "] in [" + region + "]!")

			terminal.Information("Done!")
		} else {
			terminal.Notice("Params:")
			fmt.Println(params.String())
		}
	}

	// Create the Alarms and Scaling Policies
	if len(cfg.Alarms) > 0 {

		terminal.Delta("Creating CloudWatch Alarms.")

		for _, alarm := range cfg.Alarms {

			alarmCfg, err := config.LoadAlarmClass(alarm)
			if err != nil {
				return err
			}
			terminal.Information("Found CloudWatch Alarm class configuration for [" + alarm + "]")

			err = createAutoScaleAlarms(alarm, alarmCfg, asgList, dryRun)
			if err != nil {
				return err
			}
		}
	}

	return nil

}

// CreateAutoScaleAlarm creates a new CloudWatch Alarm given the provided class
func CreateAutoScaleAlarms(class string, asgSearch string, dryRun bool) error {

	// --dry-run flag
	if dryRun {
		terminal.Information("--dry-run flag is set, not making any actual changes!")
	}

	// Verify the alarm class input
	cfg, err := config.LoadAlarmClass(class)
	if err != nil {
		return err
	}
	terminal.Information("Found CloudWatch Alarm class configuration for [" + class + "]")

	asgList, errs := GetAutoScaleGroups(asgSearch)
	if errs != nil {
		return errors.New("Error while retrieving the list of AutoScale Groups!")
	}

	if len(*asgList) > 0 {
		// Print the table
		asgList.PrintTable()
	} else {
		return errors.New("No AutoScaling Groups found, Aborting!")
	}

	// Confirm
	if !terminal.PromptBool("Are you sure you want to create this alarm in these AutoScaling Groups?") {
		return errors.New("Aborting!")
	}

	err = createAutoScaleAlarms(class, cfg, asgList, dryRun)
	if err != nil {
		return err
	}

	terminal.Information("Done!")

	return nil
}

// private function with no terminal prompts
func createAutoScaleAlarms(name string, cfg config.AlarmClass, asgList *AutoScaleGroups, dryRun bool) (err error) {

	for _, asg := range *asgList {

		terminal.Delta("Adding Alarm [" + name + "] to AutoScale Group [" + asg.Name + "] in [" + asg.Region + "]")

		sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(asg.Region)}))
		svc := cloudwatch.New(sess)

		// Create the alarm
		params := &cloudwatch.PutMetricAlarmInput{
			AlarmName:          aws.String(name),
			ComparisonOperator: aws.String(cfg.ComparisonOperator),
			EvaluationPeriods:  aws.Int64(int64(cfg.EvaluationPeriods)),
			MetricName:         aws.String(cfg.MetricName),
			Namespace:          aws.String(cfg.Namespace),
			Period:             aws.Int64(int64(cfg.Period)),
			Statistic:          aws.String(cfg.Statistic),
			Threshold:          aws.Float64(cfg.Threshold),
			ActionsEnabled:     aws.Bool(cfg.ActionsEnabled),
			AlarmDescription:   aws.String(cfg.AlarmDescription),
			Dimensions: []*cloudwatch.Dimension{
				&cloudwatch.Dimension{
					Name:  aws.String("AutoScalingGroupName"),
					Value: aws.String(asg.Name),
				},
			},
		}

		if cfg.Unit != "" {
			params.SetUnit(cfg.Unit)
		}

		// Set the Alarm Actions
		for _, action := range cfg.AlarmActions {

			// Create the Scaling Policies
			actionCfg, err := config.LoadScalingPolicyClass(action)
			if err == nil {
				terminal.Information("Found Scaling Policy class configuration for [" + action + "]")

				alarmArn, err := createScalingPolicy(action, actionCfg, &AutoScaleGroups{asg}, dryRun)
				if err != nil {
					return err
				}
				params.AlarmActions = append(params.AlarmActions, aws.String(alarmArn))
			}
		}

		// Set the Alarm OKActions
		for _, action := range cfg.OKActions {
			params.OKActions = append(params.OKActions, aws.String(action))
		}

		// Set the Alarm InsufficientDataActions
		for _, action := range cfg.InsufficientDataActions {
			params.InsufficientDataActions = append(params.InsufficientDataActions, aws.String(action))
		}

		if !dryRun {
			_, err = svc.PutMetricAlarm(params)

			if err != nil {
				if awsErr, ok := err.(awserr.Error); ok {
					return errors.New(awsErr.Message())
				}
				return err
			}

			terminal.Delta("Created AutoScale Alarm named [" + name + "] in [" + asg.Region + "]")
		} else {
			terminal.Notice("Params:")
			fmt.Println(params.String())
		}

	}

	return nil
}

// UpdateAutoScaleGroups updates existing AutoScale Groups that match the given search term to the provided version of Launch Configuration
func UpdateAutoScaleGroups(name, version string, double, forceYes, dryRun bool) (err error) {

	// --dry-run flag
	if dryRun {
		terminal.Information("--dry-run flag is set, not making any actual changes!")
	}

	// --double flag
	if double {
		terminal.Information("--double flag is set, doubling desired and max counts!")
	}

	asgList, _ := GetAutoScaleGroups(name)

	if len(*asgList) > 0 {
		// Print the table
		asgList.PrintTable()
	} else {
		return errors.New("No AutoScaling Groups found, Aborting!")
	}

	// Confirm
	if !forceYes && !terminal.PromptBool("Are you sure you want to update these AutoScaling Groups?") {
		return errors.New("Aborting!")
	}

	// Update 'Em
	err = updateAutoScaleGroups(asgList, version, double, dryRun)
	if err == nil {
		terminal.Information("Done!")
	}

	return
}

// Private function without the confirmation terminal prompts
func updateAutoScaleGroups(asgList *AutoScaleGroups, version string, double, dryRun bool) (err error) {

	for _, asg := range *asgList {

		// Get the ASG class config
		cfg, err := config.LoadAutoscalingGroupClass(asg.Class)
		if err != nil {
			return err
		}

		// --double flag
		if double {
			cfg.DesiredCapacity = asg.DesiredCapacity * 2
			cfg.MaxSize = asg.MaxSize * 2
		}

		terminal.Information("Found Autoscaling group class configuration for [" + asg.Class + "]")

		// Get the Launch Configuration class config
		launchConfigurationCfg, err := config.LoadLaunchConfigurationClass(cfg.LaunchConfigurationClass)
		if err != nil {
			return err
		}

		// Set passed version early
		if version != "" {
			lcVer, err := strconv.Atoi(version)
			if err != nil {
				return err
			}
			launchConfigurationCfg.Version = lcVer
			terminal.Information(fmt.Sprintf("Using Launch Configuration version [%d] passed in as an argument.", launchConfigurationCfg.Version))
		}

		terminal.Information("Found Launch Configuration class configuration for [" + cfg.LaunchConfigurationClass + "]")

		// Get the AZs
		azs, errs := regions.GetAZs()
		if errs != nil {
			return errors.New("Error gathering region list")
		}

		for region, regionAZs := range azs.GetRegionMap(cfg.AvailabilityZones) {

			// TODO check if exists yet ?

			// Verify that the latest Launch Configuration is available in this region
			lcName := GetLaunchConfigurationName(region, cfg.LaunchConfigurationClass, launchConfigurationCfg.Version)
			if lcName == "" {
				return fmt.Errorf("Launch Configuration [%s] version [%d] is not available in [%s]!", cfg.LaunchConfigurationClass, launchConfigurationCfg.Version, region)
			}
			terminal.Information(fmt.Sprintf("Found Launch Configuration [%s] version [%d] in [%s]", cfg.LaunchConfigurationClass, launchConfigurationCfg.Version, asg.Region))

			sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
			svc := autoscaling.New(sess)

			params := &autoscaling.UpdateAutoScalingGroupInput{
				AutoScalingGroupName:    aws.String(asg.Name),
				DefaultCooldown:         aws.Int64(int64(cfg.DefaultCooldown)),
				DesiredCapacity:         aws.Int64(int64(cfg.DesiredCapacity)),
				HealthCheckGracePeriod:  aws.Int64(int64(cfg.HealthCheckGracePeriod)),
				HealthCheckType:         aws.String(cfg.HealthCheckType),
				LaunchConfigurationName: aws.String(lcName),
				MaxSize:                 aws.Int64(int64(cfg.MaxSize)),
				MinSize:                 aws.Int64(int64(cfg.MinSize)),
				//NewInstancesProtectedFromScaleIn: aws.Bool(true), // TODO?
				//PlacementGroup:                   aws.String("XmlStringMaxLen255"), // TODO
			}

			subList := new(Subnets)
			var vpcZones []string

			if cfg.SubnetClass != "" {
				err := GetRegionSubnets(region, subList, "")
				if err != nil {
					return err
				}
			}

			// Set the AZs
			for _, az := range regionAZs {
				if !azs.ValidAZ(az) {
					return cli.NewExitError("Availability Zone ["+az+"] is Invalid!", 1)
				}
				terminal.Information("Found Availability Zone [" + az + "]!")

				params.AvailabilityZones = append(params.AvailabilityZones, aws.String(az))

				for _, sub := range *subList {
					if sub.Class == cfg.SubnetClass && sub.AvailabilityZone == az {
						vpcZones = append(vpcZones, sub.SubnetID)
					}
				}

			}

			// Set the VPCZoneIdentifier (SubnetIds seperated by comma)
			params.VPCZoneIdentifier = aws.String(strings.Join(vpcZones, ", "))

			// Set the Termination Policies
			for _, terminationPolicy := range cfg.TerminationPolicies {
				params.TerminationPolicies = append(params.TerminationPolicies, aws.String(terminationPolicy)) // ??
			}

			// Update it!
			if !dryRun {
				_, err := svc.UpdateAutoScalingGroup(params)
				if err != nil {
					if awsErr, ok := err.(awserr.Error); ok {
						return errors.New(awsErr.Message())
					}
					return err
				}

				// Update Tags
				tagParams := &autoscaling.CreateOrUpdateTagsInput{
					Tags: []*autoscaling.Tag{
						{
							// Name
							Key:               aws.String("Name"),
							PropagateAtLaunch: aws.Bool(true),
							ResourceId:        aws.String(asg.Name),
							ResourceType:      aws.String("auto-scaling-group"),
							Value:             aws.String(lcName),
						},
						{
							// Class
							Key:               aws.String("Class"),
							PropagateAtLaunch: aws.Bool(true),
							ResourceId:        aws.String(asg.Name),
							ResourceType:      aws.String("auto-scaling-group"),
							Value:             aws.String(cfg.LaunchConfigurationClass),
						},
					},
				}

				_, err = svc.CreateOrUpdateTags(tagParams)
				if err != nil {
					if awsErr, ok := err.(awserr.Error); ok {
						return errors.New(awsErr.Message())
					}
					return err
				}

				terminal.Delta("Updated AutoScaling Group [" + asg.Name + "] in [" + region + "]!")

			} else {
				terminal.Notice("Params:")
				fmt.Println(params)
			}
		}

		// Create the Alarms and Scaling Policies
		if len(cfg.Alarms) > 0 {

			asgList := &AutoScaleGroups{
				AutoScaleGroup{
					Name:   asg.Name,
					Region: asg.Region,
				},
			}

			for _, alarm := range cfg.Alarms {

				alarmCfg, err := config.LoadAlarmClass(alarm)
				if err != nil {
					return err
				}
				terminal.Information("Found CloudWatch Alarm class configuration for [" + alarm + "]")

				err = createAutoScaleAlarms(alarm, alarmCfg, asgList, dryRun)
				if err != nil {
					return err
				}
			}
		}

	}

	return nil
}

// DeleteAutoScaleGroups deletes one or more AutoScale Groups that match the provided name and optionally the provided region
func DeleteAutoScaleGroups(name, region string, force, dryRun bool) (err error) {

	// --dry-run flag
	if dryRun {
		terminal.Information("--dry-run flag is set, not making any actual changes!")
	}

	asgList := new(AutoScaleGroups)

	// Check if we were given a region or not
	if region != "" {
		err = GetRegionAutoScaleGroups(region, asgList, name)
	} else {
		asgList, _ = GetAutoScaleGroups(name)
	}

	if err != nil {
		return errors.New("Error gathering AutoScaling Groups list")
	}

	if len(*asgList) > 0 {
		// Print the table
		asgList.PrintTable()
	} else {
		return errors.New("No AutoScaling Groups found, Aborting!")
	}

	// Confirm
	if !terminal.PromptBool("Are you sure you want to delete these AutoScaling Groups?") {
		return errors.New("Aborting!")
	}

	// Delete 'Em

	err = deleteAutoScaleGroups(asgList, force, dryRun)
	if err != nil {
		return err
	}

	terminal.Information("Done!")

	return nil
}

// Private function without the confirmation terminal prompts
func deleteAutoScaleGroups(asgList *AutoScaleGroups, force, dryRun bool) (err error) {
	for _, asg := range *asgList {
		sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(asg.Region)}))
		svc := autoscaling.New(sess)

		params := &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: aws.String(asg.Name),
			ForceDelete:          aws.Bool(force),
		}

		// Delete it!
		if !dryRun {
			_, err := svc.DeleteAutoScalingGroup(params)
			if err != nil {
				if awsErr, ok := err.(awserr.Error); ok {
					return errors.New(awsErr.Message())
				}
				return err
			}

			terminal.Delta("Deleted AutoScaling Group [" + asg.Name + "] in [" + asg.Region + "]!")

		} else {
			terminal.Notice("Params:")
			fmt.Println(params)
		}

	}

	return nil
}

// SuspendProcesses suspends AutoScaling actions on AutoScaling Groups that match the provided search term and (optional) region
func SuspendProcesses(search, region string, dryRun bool) (err error) {

	// --dry-run flag
	if dryRun {
		terminal.Information("--dry-run flag is set, not making any actual changes!")
	}

	asgList := new(AutoScaleGroups)

	// Check if we were given a region or not
	if region != "" {
		err = GetRegionAutoScaleGroups(region, asgList, search)
	} else {
		asgList, _ = GetAutoScaleGroups(search)
	}

	if err != nil {
		return errors.New("Error gathering Autoscale Group list")
	}

	if len(*asgList) > 0 {
		// Print the table
		asgList.PrintTable()
	} else {
		return errors.New("No Autoscale Groups found!")
	}

	// Confirm
	if !terminal.PromptBool("Are you sure you want to suspend these Autoscale Groups?") {
		return errors.New("Aborting!")
	}

	// Suspend 'Em
	if !dryRun {
		err = suspendProcesses(asgList)
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				return errors.New(awsErr.Message())
			}
			return err
		}

		terminal.Information("Done!")
	}

	return nil
}

func suspendProcesses(asgList *AutoScaleGroups) error {
	for _, asg := range *asgList {
		sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(asg.Region)}))
		svc := autoscaling.New(sess)

		params := &autoscaling.ScalingProcessQuery{
			AutoScalingGroupName: aws.String(asg.Name),
		}
		_, err := svc.SuspendProcesses(params)

		if err != nil {
			return err
		}

		terminal.Delta("Suspended processes on Autoscale Group [" + asg.Name + "] in [" + asg.Region + "]!")
	}

	return nil
}

// ResumeProcesses resumes AutoScaling actions on AutoScaling Groups that match the provided search term and (optional) region
func ResumeProcesses(search, region string, dryRun bool) (err error) {

	// --dry-run flag
	if dryRun {
		terminal.Information("--dry-run flag is set, not making any actual changes!")
	}

	asgList := new(AutoScaleGroups)

	// Check if we were given a region or not
	if region != "" {
		err = GetRegionAutoScaleGroups(region, asgList, search)
	} else {
		asgList, _ = GetAutoScaleGroups(search)
	}

	if err != nil {
		return errors.New("Error gathering Autoscale Group list")
	}

	if len(*asgList) > 0 {
		// Print the table
		asgList.PrintTable()
	} else {
		return errors.New("No Autoscale Groups found!")
	}

	// Confirm
	if !terminal.PromptBool("Are you sure you want to resume these Autoscale Groups?") {
		return errors.New("Aborting!")
	}

	// Resume 'Em
	if !dryRun {
		err = resumeProcesses(asgList)
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				return errors.New(awsErr.Message())
			}
			return err
		}

		terminal.Information("Done!")
	}

	return
}

func resumeProcesses(asgList *AutoScaleGroups) error {
	for _, asg := range *asgList {
		sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(asg.Region)}))
		svc := autoscaling.New(sess)

		params := &autoscaling.ScalingProcessQuery{
			AutoScalingGroupName: aws.String(asg.Name),
		}
		_, err := svc.ResumeProcesses(params)

		if err != nil {
			return err
		}

		terminal.Delta("Resumed processes on Autoscale Group [" + asg.Name + "] in [" + asg.Region + "]!")
	}

	return nil
}

// PrintTable Prints an ascii table of the list of AutoScaling Groups
func (a *AutoScaleGroups) PrintTable() {
	if len(*a) == 0 {
		terminal.ShowErrorMessage("Warning", "No Autoscale Groups Found!")
		return
	}

	var header []string
	rows := make([][]string, len(*a))

	for index, asg := range *a {
		models.ExtractAwsmTable(index, asg, &header, &rows)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(header)
	table.AppendBulk(rows)
	table.Render()
}

// PrintTable Prints an ascii table of the list of AutoScaling Groups
func (s *ScalingActivities) PrintTable() {
	if len(*s) == 0 {
		terminal.ShowErrorMessage("Warning", "No Autoscale Activities Found!")
		return
	}

	var header []string
	rows := make([][]string, len(*s))

	for index, asg := range *s {
		models.ExtractAwsmTable(index, asg, &header, &rows)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(header)
	table.AppendBulk(rows)
	table.Render()
}
