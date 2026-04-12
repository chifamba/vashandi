package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type Queue struct {
	client *redis.Client
}

func NewQueue() *Queue {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Error("Failed to parse redis URL", "error", err)
		return nil
	}

	client := redis.NewClient(opt)
	return &Queue{
		client: client,
	}
}

// Enqueue adds a task payload to the specified list
func (q *Queue) Enqueue(ctx context.Context, taskType string, payload []byte) error {
	queueName := fmt.Sprintf("queue:%s", taskType)
	return q.client.LPush(ctx, queueName, payload).Err()
}

// StartWorker starts a simple worker loop consuming from a specific queue
func (q *Queue) StartWorker(ctx context.Context, taskType string, handler func([]byte) error) {
	queueName := fmt.Sprintf("queue:%s", taskType)

	go func() {
		slog.Info("Starting worker", "queue", queueName)
		for {
			select {
			case <-ctx.Done():
				slog.Info("Worker shutting down", "queue", queueName)
				return
			default:
				// BRPOP blocks until an item is available or timeout (2 seconds)
				result, err := q.client.BRPop(ctx, 2*time.Second, queueName).Result()
				if err == redis.Nil {
					// Timeout occurred, loop again
					continue
				} else if err != nil {
					// Other errors or context cancelled
					if ctx.Err() != nil {
						return
					}
					slog.Error("Error popping from queue", "queue", queueName, "error", err)
					time.Sleep(1 * time.Second) // backoff
					continue
				}

				if len(result) == 2 {
					payload := []byte(result[1])
					if err := handler(payload); err != nil {
						slog.Error("Task handler failed", "queue", queueName, "error", err)
						// Basic retry or DLQ logic would go here
					}
				}
			}
		}
	}()
}

// Example Handlers for Vashandi OpenBrain integration
func HandleTierPromotion(payload []byte) error {
	slog.Info("Processing tier_promotion", "payload", string(payload))
	// logic to analyze memory deduplication and promote tiers
	return nil
}

func HandleIngestBatch(payload []byte) error {
	slog.Info("Processing ingest_batch", "payload", string(payload))
	// logic to calculate embeddings and store into pgvector
	return nil
}
