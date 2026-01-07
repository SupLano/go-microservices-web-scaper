package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/net/html"
)

func (v *Visisted) CheckAndMark(url string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.history[url]; ok {
		return true
	}
	v.history[url] = true
	return false
}

type Visisted struct {
	mu sync.Mutex
	history map[string]bool
}

type CrawelerEngine struct {
	visited *Visisted
	urls chan struct{}
	wg sync.WaitGroup
}

func (c *CrawelerEngine) crawl(url string, source string, depth int) {
	if depth < 0 || c.visited.CheckAndMark(url) {
		return
	}
	log.Printf("[CRAWL] Visiting: %s | Source: %s\n", url, source)

	c.urls <- struct{}{}

	c.wg.Add(1)
	
	go func() {		
		defer c.wg.Done()
		defer func() { <-c.urls }()

		links, err := extractLinks(url)

		if err != nil {
			log.Printf("[ERROR] Failed to fetch %s: %v\n", url, err)
			return
		}
		for _, link := range links {
			c.crawl(link, url, depth-1)
		}
	}()
}

func main() {
	
	urls := make(chan struct{}, 2)
	engine := &CrawelerEngine{
		visited: &Visisted{
			history: make(map[string]bool),
		},
		urls: urls,
	}
	engine.crawl("https://www.boot.dev", "SEED", 2)
	engine.wg.Wait()
	log.Println("Crawl Complete.")
}

func extractLinks(baseTarget string) ([]string, error) {
	resp, err := http.Get(baseTarget)
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







