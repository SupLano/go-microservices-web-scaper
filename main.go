package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"encoding/json"

	"golang.org/x/net/html"
)
func (v *Crawler) CheckAndMark(u string) bool {
	added, err := v.redisClient.client.SAdd(context.Background(), "visited_urls", u).Result()
	if err != nil {
		log.Printf("Redis error calling SAdd: %v", err)
		// If Redis fails, we might want to default to "visited" (true) to avoid infinite loops,
		// or "not visited" (false) to keep trying. 
		// "true" is safer to prevent runaway crawling.
		return true 
	}
	// If added == 1, it was New. We want to return false (not visited).
	// If added == 0, it was Already there. We want to return true (visited).
	return added == 0
}


// --- ENGINE LAYER ---

// WorkItem carries the state through the heap-based channel.
type WorkItem struct {
	URL   string
	Depth int
}

type Crawler struct {
	redisClient *RedisClient
	wg          sync.WaitGroup
}

func (c *Crawler) Start(seedURL string, maxDepth int, workerCount int) {
	done := make(chan struct{})

	// Coordinator: Watches the WaitGroup and signals completion
	go func() {
		c.wg.Wait()
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Seed the first task
	c.wg.Add(1)
	data, _ := json.Marshal(map[string]interface{}{"url": seedURL, "depth": maxDepth})
	c.redisClient.client.LPush(ctx, "jobs", data)

	// Spawn the Worker Pool
	for i := 0; i < workerCount; i++ {
		go c.worker()
	}

	// Block until all work is complete
	<-done
}

func (c *Crawler) worker() {
	// Each worker pulls jobs from Redis queue in an infinite loop
	for {
		result, err := c.redisClient.client.BRPop(context.Background(), 0, "jobs").Result()
		if err != nil {
			// Handle connection drops or timeouts
			fmt.Printf("Redis error: %v\n", err)
			time.Sleep(time.Second)
			continue
		}
		
		// BRPop returns []string{key_name, value}
		rawJSON := result[1]

		var item WorkItem
		if err := json.Unmarshal([]byte(rawJSON), &item); err != nil {
			fmt.Printf("Error unmarshaling job: %v\n", err)
			c.wg.Done()
			continue
		}

		c.process(item)
		c.wg.Done()
	}
}
func (c *Crawler) process(item WorkItem) {
	// Base Cases: Depth limit or already visited
	if item.Depth <= 0 || c.CheckAndMark(item.URL) {
		return
	}

	fmt.Printf("[Depth %d] Crawling: %s\n", item.Depth, item.URL)

	timeoutContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	links, err := extractLinks(timeoutContext, item.URL)
	if err != nil {
		return
	}

	for _, link := range links {
		c.wg.Add(1)
		data, _ := json.Marshal(map[string]interface{}{"url": link, "depth": item.Depth - 1})
		c.redisClient.client.LPush(context.Background(), "jobs", data)
	}
}
func main() {
	// Define CLI flags
	url := flag.String("url", "", "Seed URL to start crawling (required)")
	depth := flag.Int("depth", 3, "Maximum crawl depth")
	workers := flag.Int("workers", 10, "Number of concurrent workers")
	redisAddr := flag.String("redis-addr", "localhost:6379", "Redis server address")
	
	flag.Parse()
	
	// Validate required flags
	if *url == "" {
		fmt.Println("Error: --url flag is required")
		flag.Usage()
		return
	}
	
	// Validate depth
	if *depth <= 0 {
		fmt.Println("Error: --depth must be greater than 0")
		return
	}
	
	// Validate workers
	if *workers <= 0 {
		fmt.Println("Error: --workers must be greater than 0")
		return
	}
	
	start := time.Now()
	redisClient := NewRedisClient(*redisAddr)
	defer redisClient.CloseConnection()

	crawler := &Crawler{
		redisClient: redisClient,
	}

	fmt.Printf("Starting crawler...\n")
	fmt.Printf("URL: %s\n", *url)
	fmt.Printf("Max Depth: %d\n", *depth)
	fmt.Printf("Workers: %d\n", *workers)
	fmt.Printf("Redis: %s\n\n", *redisAddr)

	crawler.Start(*url, *depth, *workers)

	fmt.Printf("\n--- Crawl Complete ---\n")
	fmt.Printf("Duration: %v\n", time.Since(start))
	
	// Get count from Redis
	count, _ := redisClient.client.SCard(context.Background(), "visited_urls").Result()
	fmt.Printf("Unique Pages Found: %d\n", count)
}

func extractLinks(ctx context.Context, baseTarget string) ([]string, error) {
	req, err := http.NewRequest("GET", baseTarget, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status error: %d", resp.StatusCode)
	}

	// Parse the base URL once to resolve relative links (e.g., "/about" -> "https://site.com/about")
	base, err := url.Parse(baseTarget)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var links []string
	// ITERATIVE STACK: We manage the stack ourselves to prevent deep recursion issues
	// We pre-allocate a small slice to hold nodes
	stack := make([]*html.Node, 0, 50)
	stack = append(stack, doc)

	for len(stack) > 0 {
		// Pop the last node
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					resolved := resolveURL(base, a.Val)
					if resolved != "" {
						links = append(links, resolved)
					}
					break
				}
			}
		}

		// Add children to the stack for processing
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			stack = append(stack, c)
		}
		
		// Optimization: stop if we have enough links for this branch
		if len(links) >= 10 { 
			break 
		}
	}

	return links, nil
}

func resolveURL(base *url.URL, href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}







