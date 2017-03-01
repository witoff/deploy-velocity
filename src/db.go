package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"strconv"
)

func GetDDB() *dynamodb.DynamoDB {
	session, err := session.NewSession()
	if err != nil {
		fmt.Println("failed to create dynamo session,", err)
		return nil
	}

	ddb := dynamodb.New(session)
	return ddb
}

func GetLastVersion(ddb *dynamodb.DynamoDB, host string) (string, int) {
	params := &dynamodb.QueryInput{
		TableName:              aws.String("deploy-velocity"),
		ProjectionExpression:   aws.String("version_hash,update_count"),
		ConsistentRead:         aws.Bool(false),
		Limit:                  aws.Int64(1),
		ScanIndexForward:       aws.Bool(false),
		KeyConditionExpression: aws.String("host = :host"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":host": {
				S: aws.String(host),
			},
		},
	}
	resp, err := ddb.Query(params)

	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		fmt.Println(err.Error())
		return "ddb_error", 0
	}

	// Pretty-print the response data.
	if len((*resp).Items) < 1 {
		return "no_data", 0
	}
	if resp.Items[0]["version_hash"] == nil {
		return "bad_data", 0
	}

	version_hash := *resp.Items[0]["version_hash"].S
	update_count := 0
	if resp.Items[0]["update_count"] != nil {
		update_count, err = strconv.Atoi(*resp.Items[0]["update_count"].N)
	}
	return version_hash, update_count
}

func UpdateVersion(ddb *dynamodb.DynamoDB, v *Version, update_count int) {
	params := &dynamodb.PutItemInput{
		Item: map[string]*dynamodb.AttributeValue{
			"host":          {S: aws.String(v.host)},
			"updated":       {N: aws.String(strconv.Itoa(int(v.updated)))},
			"update_count":  {N: aws.String(strconv.Itoa(update_count))},
			"url":           {S: aws.String(v.url)},
			"version_hash":  {S: aws.String(v.version_hash)},
			"header_hash":   {S: aws.String(v.header_hash)},
			"includes_hash": {S: aws.String(v.includes_hash)},
			"includes_list": {S: aws.String(v.includes_list)},
		},
		TableName: aws.String("deploy-velocity"),
	}
	_, err := ddb.PutItem(params)

	if err != nil {
		fmt.Println(err.Error())
	}
}
