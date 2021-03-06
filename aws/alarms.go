package aws

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/murdinc/awsm/aws/regions"
	"github.com/murdinc/awsm/config"
	"github.com/murdinc/awsm/models"
	"github.com/murdinc/terminal"
	"github.com/olekukonko/tablewriter"
)

// Alarms represents a slice of CloudWatch Alarms
type Alarms []Alarm

// Alarm represents a single CloudWatch Alarm
type Alarm models.Alarm

// GetAlarms returns a slice of CloudWatch Alarms based on the given search term
func GetAlarms(search string) (*Alarms, []error) {
	var wg sync.WaitGroup
	var errs []error

	alList := new(Alarms)
	regions := GetRegionListWithoutIgnored()

	for _, region := range regions {
		wg.Add(1)

		go func(region *ec2.Region) {
			defer wg.Done()
			err := GetRegionAlarms(*region.RegionName, alList, search)
			if err != nil {
				terminal.ShowErrorMessage(fmt.Sprintf("Error gathering alarm list for region [%s]", *region.RegionName), err.Error())
				errs = append(errs, err)
			}
		}(region)
	}
	wg.Wait()

	return alList, errs
}

// GetRegionAlarms returns a list of CloudWatch Alarms for the given region into the provided Alarms slice
func GetRegionAlarms(region string, alList *Alarms, search string) error {
	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
	svc := cloudwatch.New(sess)

	result, err := svc.DescribeAlarms(&cloudwatch.DescribeAlarmsInput{})
	if err != nil {
		return err
	}

	al := make(Alarms, len(result.MetricAlarms))
	for i, alarm := range result.MetricAlarms {
		al[i].Marshal(alarm, region)
	}

	if search != "" {
		term := regexp.MustCompile(search)
	Loop:
		for i, g := range al {
			rAsg := reflect.ValueOf(g)

			for k := 0; k < rAsg.NumField(); k++ {
				sVal := rAsg.Field(k).String()

				if term.MatchString(sVal) {
					*alList = append(*alList, al[i])
					continue Loop
				}
			}
		}
	} else {
		*alList = append(*alList, al[:]...)
	}

	return nil
}

// Marshal parses the response from the aws sdk into an awsm Alarm
func (a *Alarm) Marshal(alarm *cloudwatch.MetricAlarm, region string) {
	var dimensions []string
	var operator string

	for _, dim := range alarm.Dimensions {
		dimensions = append(dimensions, aws.StringValue(dim.Name)+" = "+aws.StringValue(dim.Value))
	}

	switch aws.StringValue(alarm.ComparisonOperator) {
	case "GreaterThanThreshold":
		operator = ">"

	case "GreaterThanOrEqualToThreshold":
		operator = ">="

	case "LessThanThreshold":
		operator = "<"

	case "LessThanOrEqualToThreshold":
		operator = "<="
	}

	var actionArns []string
	var actionNames []string

	for _, action := range alarm.AlarmActions {
		arnStr := aws.StringValue(action)
		actionArns = append(actionArns, arnStr)

		arn, err := ParseArn(arnStr)
		if err == nil {
			actionNames = append(actionNames, arn.PolicyName)
		} else {
			actionNames = append(actionNames, "??????")
		}
	}

	a.Name = aws.StringValue(alarm.AlarmName)
	a.Arn = aws.StringValue(alarm.AlarmArn)
	a.Description = aws.StringValue(alarm.AlarmDescription)
	a.State = aws.StringValue(alarm.StateValue)
	a.Trigger = fmt.Sprintf("%s %s %d (%s)", aws.StringValue(alarm.MetricName), operator, int(aws.Float64Value(alarm.Threshold)), aws.StringValue(alarm.Statistic))
	a.Period = fmt.Sprint(aws.Int64Value(alarm.Period))
	a.EvalPeriods = fmt.Sprint(aws.Int64Value(alarm.EvaluationPeriods))
	a.ActionArns = actionArns
	a.ActionNames = strings.Join(actionNames, ", ")
	a.Dimensions = strings.Join(dimensions, ", ")
	a.Namespace = aws.StringValue(alarm.Namespace)
	a.Region = region
}

// CreateAlarm creates a new CloudWatch Alarm given the provided class and region
func CreateAlarm(class string, region string, dryRun bool) error {

	// --dry-run flag
	if dryRun {
		terminal.Information("--dry-run flag is set, not making any actual changes!")
	}

	// Validate the region
	if !regions.ValidRegion(region) {
		return errors.New("Region [" + region + "] is Invalid!")
	}

	// Verify the alarm class input
	cfg, err := config.LoadAlarmClass(class)
	if err != nil {
		return err
	}
	terminal.Information("Found CloudWatch Alarm class configuration for [" + class + "]")

	return createAlarm(class, cfg, region, dryRun)
}

// private function with no terminal prompts
func createAlarm(name string, cfg config.AlarmClass, region string, dryRun bool) (err error) {

	sess := session.Must(session.NewSession(&aws.Config{Region: aws.String(region)}))
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
	}

	if cfg.Unit != "" {
		params.SetUnit(cfg.Unit)
	}

	// Set the Alarm Actions
	for _, action := range cfg.AlarmActions {
		params.AlarmActions = append(params.AlarmActions, aws.String(action))
	}

	// Set the Alarm OKActions
	for _, action := range cfg.OKActions {
		params.OKActions = append(params.OKActions, aws.String(action))
	}

	// Set the Alarm InsufficientDataActions
	for _, action := range cfg.InsufficientDataActions {
		params.InsufficientDataActions = append(params.InsufficientDataActions, aws.String(action))
	}

	/*
		// Set the Alarm Dimensions
		for name, value := range dimensions {
			params.Dimensions = append(params.Dimensions, &cloudwatch.Dimension{
				Name:  aws.String(name),
				Value: aws.String(value),
			})
		}
	*/

	if !dryRun {
		_, err = svc.PutMetricAlarm(params)

		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				return errors.New(awsErr.Message())
			}
			return err
		}

		terminal.Delta("Created Alarm named [" + name + "] in [" + region + "]")

	}

	terminal.Information("Done!")

	return nil
}

// PrintTable Prints an ascii table of the list of CloudWatch Alarms
func (i *Alarms) PrintTable() {
	if len(*i) == 0 {
		terminal.ShowErrorMessage("Warning", "No Alarms Found!")
		return
	}

	var header []string
	rows := make([][]string, len(*i))

	for index, alarm := range *i {
		models.ExtractAwsmTable(index, alarm, &header, &rows)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(header)
	table.AppendBulk(rows)
	table.Render()
}
