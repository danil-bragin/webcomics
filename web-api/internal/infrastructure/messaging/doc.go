// Package messaging holds broker adapters beyond the outbox relay (e.g. direct
// publishers for non-domain integration events, or alternative transports).
// The Redis Streams publisher/subscriber factories live in platform/redis;
// this package is the place for higher-level messaging concerns if they grow.
package messaging
