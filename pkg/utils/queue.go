package utils

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var queueTimeHistogramVec = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "queue_waiting_time",
		Help:    "elastic queue waiting time distribution in seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 3, 4, 5, 7.5, 10, 20, 30, 60, 120, 180, 240, 300, 500, 1000},
	},
	[]string{"name"},
)

var queueLength = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "queue_length",
		Help: "elastic queue length",
	},
	[]string{"name"},
)

type ElasticQueue[T any] struct {
	input     chan T
	output    chan T
	name      string
	maxLength int
}

type Options struct {
	MaxLength         int
	InputQueueChanLen int
}

type Option func(*Options)

func WithMaxLength(maxLength int) Option {
	return func(o *Options) {
		o.MaxLength = maxLength
	}
}

func WithInputChanLen(chanLen int) Option {
	return func(o *Options) {
		o.InputQueueChanLen = chanLen
	}
}

func NewQueue[T any](name string, opts ...Option) *ElasticQueue[T] {
	options := Options{
		MaxLength: 0,
	}
	for _, opt := range opts {
		opt(&options)
	}
	return &ElasticQueue[T]{
		name:      name,
		input:     make(chan T, options.InputQueueChanLen),
		output:    make(chan T),
		maxLength: options.MaxLength,
	}
}

func (q *ElasticQueue[T]) Input() chan<- T {
	return q.input
}

func (q *ElasticQueue[T]) Output() <-chan T {
	return q.output
}

type message[T any] struct {
	recv  time.Time
	value T
}

func (q *ElasticQueue[T]) Run(ctx context.Context) {
	// TODO: optimize storage to avoid memory allocations/reallocations
	var msgs []message[T]
	for {
		if len(msgs) == 0 {
			select {
			case <-ctx.Done():
				return
			case msg := <-q.input:
				msgs = append(msgs, message[T]{value: msg, recv: time.Now()})
			}
		}

		input := q.input
		// maxLength = 0 means a queue is unlimited
		if q.maxLength > 0 && q.maxLength <= len(msgs) {
			input = nil
		}

		select {
		case <-ctx.Done():
			return
		case msg := <-input:
			msgs = append(msgs, message[T]{value: msg, recv: time.Now()})
		case q.output <- msgs[0].value:
			delta := time.Since(msgs[0].recv)
			msgs = msgs[1:]
			queueTimeHistogramVec.WithLabelValues(q.name).Observe(delta.Seconds())
		}
		queueLength.WithLabelValues(q.name).Set(float64(len(msgs)))
	}
}
