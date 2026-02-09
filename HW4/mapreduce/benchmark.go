package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

func main() {
	re := regexp.MustCompile(`[a-zA-Z]+`)
	files := []string{"input.txt", "input_5x.txt", "input_10x.txt", "input_20x.txt"}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("%s: not found, skipping\n", f)
			continue
		}
		start := time.Now()
		counts := map[string]int{}
		for _, word := range re.FindAllString(string(data), -1) {
			counts[strings.ToLower(word)]++
		}
		elapsed := time.Since(start)
		fmt.Printf("%s: %d unique words, %v\n", f, len(counts), elapsed)
	}
}