package eeyore

import (
	"fmt"
	"github.com/Shopify/sarama"
	"log"
	"os"
	"time"
)

const LOG_INFO int = 1
const LOG_WARNING int = 2
const LOG_ERROR int = 3

type KafkaConfig struct {
	Host  []string
	Topic string
}

type LogEntry struct {
	Message string
	Level   int

	encoded []byte
	err     error
}

type KafkaLogger struct {
	producer sarama.AsyncProducer
	topic    string
}

func (kl *KafkaLogger) Init(kafka_config KafkaConfig) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForLocal
	config.Producer.Retry.Max = 3

	producer, err := sarama.NewAsyncProducer(
		kafka_config.Host, config)

	if err != nil {
		log.Fatalln("Failed to start Sarama producer:", err)
		return
	}

	kl.producer = producer
	kl.topic = kafka_config.Topic

	go func() {
		for err := range kl.producer.Errors() {
			log.Println("Failed to write log:", err)
		}
	}()
}

func (kl *KafkaLogger) Log(level int, message string) {
	entry := &LogEntry{
		Message: message,
		Level:   level,
	}
	kl.producer.Input() <- &sarama.ProducerMessage{
		Topic: kl.topic,
		Value: entry,
	}
}

func (kl *KafkaLogger) Info(message string) {
	entry := &LogEntry{
		Message: message,
		Level:   LOG_INFO,
	}
	kl.producer.Input() <- &sarama.ProducerMessage{
		Topic: kl.topic,
		Value: entry,
	}
}

func (kl *KafkaLogger) Warning(message string) {
	entry := &LogEntry{
		Message: message,
		Level:   LOG_WARNING,
	}
	kl.producer.Input() <- &sarama.ProducerMessage{
		Topic: kl.topic,
		Value: entry,
	}
}

func (kl *KafkaLogger) Error(message string) {
	entry := &LogEntry{
		Message: message,
		Level:   LOG_ERROR,
	}
	kl.producer.Input() <- &sarama.ProducerMessage{
		Topic: kl.topic,
		Value: entry,
	}
}

func (entry *LogEntry) ensureEncoded() {
	if entry.encoded == nil {
		now := time.Now()
		hostname, _ := os.Hostname()
		entry.encoded = []byte(
			fmt.Sprintf(
				"[%.26s %-7s %s] %+v",
				now, entry.getLevel(), hostname, entry.Message))
	}
}

func (entry *LogEntry) getLevel() string {
	rv := ""
	if entry.Level == LOG_INFO {
		return "INFO"
	}
	if entry.Level == LOG_WARNING {
		return "WARNING"
	}
	if entry.Level == LOG_ERROR {
		return "ERROR"
	}
	return rv
}

func (entry *LogEntry) Length() int {
	entry.ensureEncoded()
	return len(entry.encoded)
}

func (entry *LogEntry) Encode() ([]byte, error) {
	entry.ensureEncoded()
	return entry.encoded, entry.err
}
