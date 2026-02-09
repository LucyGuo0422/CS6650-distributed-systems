package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var bucket = "mapreduce-bucket-lucy"

func main() {
	cfg, _ := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-west-2"))
	client := s3.NewFromConfig(cfg)

	http.HandleFunc("/split", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			key = "input.txt"
		}

		out, err := client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer out.Body.Close()
		data, _ := io.ReadAll(out.Body)
		words := strings.Fields(string(data))

		chunkSize := len(words) / 3
		keys := []string{}
		for i := 0; i < 3; i++ {
			start := i * chunkSize
			end := (i + 1) * chunkSize
			if i == 2 {
				end = len(words)
			}
			chunk := strings.Join(words[start:end], " ")
			ck := fmt.Sprintf("chunks/chunk_%d.txt", i)
			client.PutObject(context.TODO(), &s3.PutObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(ck),
				Body:   strings.NewReader(chunk),
			})
			keys = append(keys, ck)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"chunk_keys": keys})
	})

	fmt.Println("Splitter running on :8080")
	http.ListenAndServe(":8080", nil)
}