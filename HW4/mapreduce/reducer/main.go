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

	http.HandleFunc("/reduce", func(w http.ResponseWriter, r *http.Request) {
		keysParam := r.URL.Query().Get("keys")
		if keysParam == "" {
			http.Error(w, "missing keys param", 400)
			return
		}

		final := map[string]int{}
		for _, key := range strings.Split(keysParam, ",") {
			out, err := client.GetObject(context.TODO(), &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(strings.TrimSpace(key)),
			})
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			data, _ := io.ReadAll(out.Body)
			out.Body.Close()

			counts := map[string]int{}
			json.Unmarshal(data, &counts)
			for word, count := range counts {
				final[word] += count
			}
		}

		jsonData, _ := json.Marshal(final)
		finalKey := "final/word_counts.json"
		client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(finalKey),
			Body:   strings.NewReader(string(jsonData)),
		})

		json.NewEncoder(w).Encode(map[string]interface{}{
			"final_key":    finalKey,
			"unique_words": len(final),
		})
	})

	fmt.Println("Reducer running on :8080")
	http.ListenAndServe(":8080", nil)
}