package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Producer Kafka生产者
type Producer struct {
	writer *kafkago.Writer
	log    *zap.Logger
}

// NewProducer 创建Kafka生产者
func NewProducer(brokers []string, topic string, log *zap.Logger) *Producer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafkago.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		Async:        true,
	}
	return &Producer{writer: w, log: log}
}

// Send 发送消息
func (p *Producer) Send(ctx context.Context, key, value []byte) error {
	err := p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   key,
		Value: value,
		Time:  time.Now(),
	})
	if err != nil {
		p.log.Error("kafka send failed", zap.Error(err))
	}
	return err
}

// Close 关闭生产者
func (p *Producer) Close() error {
	return p.writer.Close()
}

// Consumer Kafka消费者
type Consumer struct {
	reader   *kafkago.Reader
	handler  func(ctx context.Context, msg kafkago.Message) error
	log      *zap.Logger
	wg       sync.WaitGroup
	cancelFn context.CancelFunc
}

// NewConsumer 创建Kafka消费者
func NewConsumer(brokers []string, topic, groupID string, log *zap.Logger) *Consumer {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})
	return &Consumer{reader: r, log: log}
}

// SetHandler 设置消息处理函数
func (c *Consumer) SetHandler(fn func(ctx context.Context, msg kafkago.Message) error) {
	c.handler = fn
}

// Start 启动消费循环
func (c *Consumer) Start(ctx context.Context) {
	ctx, c.cancelFn = context.WithCancel(ctx)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.log.Error("kafka read error", zap.Error(err))
				continue
			}
			if c.handler != nil {
				if err := c.handler(ctx, msg); err != nil {
					c.log.Error("kafka handler error",
						zap.String("topic", msg.Topic),
						zap.Int("partition", msg.Partition),
						zap.Int64("offset", msg.Offset),
						zap.Error(err))
				}
			}
		}
	}()
	c.log.Info("kafka consumer started", zap.String("topic", c.reader.Config().Topic))
}

// Stop 停止消费者
func (c *Consumer) Stop() error {
	if c.cancelFn != nil {
		c.cancelFn()
	}
	c.wg.Wait()
	return c.reader.Close()
}

// DevProducer 开发模式空生产者（不连Kafka）
type DevProducer struct {
	log *zap.Logger
}

func NewDevProducer(log *zap.Logger) *DevProducer {
	return &DevProducer{log: log}
}

func (d *DevProducer) Send(_ context.Context, key, value []byte) error {
	d.log.Debug("dev kafka (no-op)", zap.String("key", string(key)), zap.Int("len", len(value)))
	return nil
}

func (d *DevProducer) Close() error { return nil }

// ProducerInterface 生产者接口
type ProducerInterface interface {
	Send(ctx context.Context, key, value []byte) error
	Close() error
}

// Ensure interfaces
var _ ProducerInterface = (*Producer)(nil)
var _ ProducerInterface = (*DevProducer)(nil)

// NewProducerOrDev 根据配置创建Kafka生产者，dev_no_kafka=true时返回空实现
func NewProducerOrDev(brokers []string, topic string, devMode bool, log *zap.Logger) ProducerInterface {
	if devMode {
		log.Warn("Kafka disabled (dev_no_kafka=true), using no-op producer")
		return NewDevProducer(log)
	}
	if len(brokers) == 0 || brokers[0] == "" {
		return NewDevProducer(log)
	}
	return NewProducer(brokers, topic, log)
}

// Ensure formatting
var _ = fmt.Sprintf
