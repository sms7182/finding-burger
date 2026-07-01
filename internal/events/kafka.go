package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

type OrderRoutedEvent struct {
	EventID         string    `json:"event_id"`
	OrderID         string    `json:"order_id"`
	VendorID        int       `json:"vendor_id"`
	VendorName      string    `json:"vendor_name"`
	CustomerLat     float64   `json:"customer_lat"`
	CustomerLon     float64   `json:"customer_lon"`
	CartTotal       float64   `json:"cart_total"`
	DiscountApplied bool      `json:"discount_applied"`
	DiscountAmount  float64   `json:"discount_amount"`
	FinalTotal      float64   `json:"final_total"`
	VendorLoad      int       `json:"vendor_load_at_routing"`
	EligibleCount   int       `json:"eligible_vendor_count"`
	DegradedMode    bool      `json:"degraded_mode"`
	RoutedAt        time.Time `json:"routed_at"`
	SchemaVersion   int       `json:"schema_version"`
}

type Producer struct {
	writer *kafka.Writer
	topic  string
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.Hash{}, // partition by message key (vendor id)
			RequiredAcks:           kafka.RequireOne,
			AllowAutoTopicCreation: true,
			BatchTimeout:           50 * time.Millisecond,
		},
		topic: topic,
	}
}

func (p *Producer) PublishOrderRouted(ctx context.Context, e OrderRoutedEvent) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return p.writer.WriteMessages(cctx, kafka.Message{
		Key:   []byte(strconv.Itoa(e.VendorID)),
		Value: payload,
		Time:  time.Now(),
	})
}

func (p *Producer) Close() error { return p.writer.Close() }
func EnsureTopic(brokers []string, topic string, partitions int) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		err := tryCreateTopic(brokers[0], topic, partitions)
		if err == nil {
			log.Printf("kafka topic %q ensured", topic)
			return
		}
		if time.Now().After(deadline) {
			log.Printf("could not pre-create topic %q (relying on auto-create): %v", topic, err)
			return
		}
		time.Sleep(2 * time.Second)
	}
}

func tryCreateTopic(broker, topic string, partitions int) error {
	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	ctrlConn, err := kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer ctrlConn.Close()

	return ctrlConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	})
}
