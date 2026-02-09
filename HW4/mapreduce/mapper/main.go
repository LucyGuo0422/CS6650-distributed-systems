package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var bucket = "mapreduce-bucket-lucy"
var re = regexp.MustCompile(`[a-zA-Z]+`)

func main() {
	cfg, _ := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-west-2"))
	client := s3.NewFromConfig(cfg)

	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing key param", 400)
			return
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

		counts := map[string]int{}
		for _, word := range re.FindAllString(string(data), -1) {
			counts[strings.ToLower(word)]++
		}

		// Save result
		// chunks/chunk_0.txt -> results/chunk_0.json
		parts := strings.Split(key, "/")
		name := strings.Replace(parts[len(parts)-1], ".txt", "", 1)
		resultKey := fmt.Sprintf("results/%s.json", name)

		jsonData, _ := json.Marshal(counts)
		client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(resultKey),
			Body:   strings.NewReader(string(jsonData)),
		})

		json.NewEncoder(w).Encode(map[string]interface{}{"result_key": resultKey})
	})

	fmt.Println("Mapper running on :8080")
	http.ListenAndServe(":8080", nil)
}