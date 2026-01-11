# Go Web Crawler with Redis

A concurrent web crawler built in Go that uses Redis for distributed job queuing and visited URL tracking.

## Features

- **Concurrent crawling** with configurable worker pool
- **Redis-backed architecture** for scalability and persistence
- **Depth-limited crawling** to control scope
- **Duplicate URL detection** using Redis sets
- **Graceful coordination** with WaitGroup-based synchronization

## Architecture

### Components

- **WorkItem**: Carries URL and depth information through the system
- **Crawler**: Main orchestrator managing workers and coordination
- **RedisClient**: Wrapper for Redis operations

### How It Works

1. **Job Queue**: Uses Redis list (`jobs`) as a distributed work queue
   - Producer: `LPUSH` adds new URLs to crawl
   - Consumer: `BRPOP` blocks until jobs are available

2. **Visited Tracking**: Uses Redis set (`visited_urls`) to prevent duplicate crawling
   - `SADD` atomically checks and marks URLs as visited
   - Returns whether URL was already seen

3. **Worker Pool**: Multiple goroutines process jobs concurrently
   - Each worker runs an infinite loop pulling from Redis
   - WaitGroup tracks pending work for graceful shutdown

4. **Coordinator**: Goroutine that monitors completion
   - Waits for WaitGroup to reach zero
   - Signals main thread when all work is done

## Prerequisites

- Go 1.16 or higher
- Redis server running on `localhost:6379`

## Installation

```bash
# Install dependencies
go get github.com/go-redis/redis/v8
go get golang.org/x/net/html
```

## Usage

### Start Redis Server

```bash
redis-server
```

### Run the Crawler

**Basic usage with required URL flag:**
```bash
go run main.go redis.go --url https://go.dev
```

**Custom depth and workers:**
```bash
go run main.go redis.go --url https://go.dev --depth 2 --workers 5
```

**Custom Redis address:**
```bash
go run main.go redis.go --url https://example.com --redis-addr localhost:6380
```

**Show help:**
```bash
go run main.go redis.go --help
```

### CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--url` | string | *required* | Seed URL to start crawling |
| `--depth` | int | 3 | Maximum crawl depth |
| `--workers` | int | 10 | Number of concurrent workers |
| `--redis-addr` | string | localhost:6379 | Redis server address |

### Configuration

All configuration is done via command-line flags. The crawler will validate inputs and show errors for:
- Missing `--url` flag
- Invalid depth (must be > 0)
- Invalid worker count (must be > 0)

### Clear Redis Data

```bash
redis-cli FLUSHALL
```

## Code Structure

```
.
├── main.go       # Crawler logic and entry point
└── redis.go      # Redis client wrapper
```

## How the Crawler Works

### 1. Initialization
- Creates Redis client connection
- Initializes crawler with WaitGroup

### 2. Seeding
- Marshals seed URL and depth to JSON
- Pushes to Redis `jobs` list
- Increments WaitGroup counter

### 3. Worker Processing
Each worker:
- Blocks on `BRPOP` waiting for jobs
- Unmarshals JSON payload
- Checks if URL already visited (Redis `SADD`)
- Extracts links from page
- Pushes new jobs to Redis queue
- Decrements WaitGroup

### 4. Termination
- Coordinator goroutine waits for WaitGroup to reach zero
- Signals completion to main thread
- Displays statistics (duration, unique pages)

## Example Output

```
PONG
[Depth 4] Crawling: https://go.dev
[Depth 3] Crawling: https://go.dev/doc
[Depth 3] Crawling: https://go.dev/blog
...

--- Crawl Complete ---
Duration: 15.234s
Unique Pages Found: 127
```

## Key Design Decisions

### Why Redis?

- **Persistence**: Jobs survive crashes
- **Scalability**: Can distribute across multiple machines
- **Atomic operations**: `SADD` prevents race conditions
- **Blocking operations**: `BRPOP` eliminates busy-waiting

### Concurrency Model

- **WaitGroup**: Tracks pending work without explicit counting
- **Buffered channels replaced by Redis**: Eliminates memory constraints
- **Worker pool**: Fixed number of goroutines prevents resource exhaustion

### Error Handling

- Redis errors: Log and retry after delay
- JSON unmarshal errors: Skip job and continue
- HTTP errors: Skip URL and continue

## Limitations

- No robots.txt checking
- No rate limiting per domain
- No URL normalization (may visit same page with different query params)
- Hard-coded 10-link limit per page

## Future Improvements

- [ ] Add configurable rate limiting
- [ ] Implement robots.txt compliance
- [ ] Add URL normalization
- [ ] Support for graceful shutdown (SIGINT handling)
- [ ] Metrics and monitoring (Prometheus)
- [ ] Configurable Redis connection settings
- [ ] Domain-specific crawling rules
