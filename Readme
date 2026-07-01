1-docker compose up -d
2:
   When an incoming order falls within overlapping vendor delivery zones, the system executes a two-tier resolution strategy:
       Primary Logic: The system reads the active order counters directly from Redis (HGet) and routes the order to the vendor with the lowest active load in O(1) time
       Tie-Breaker: If two or more matching vendors share the same load count, the routing engine queries the database to select the vendor whose geometric centroid is closest to the client's coordinates.  
       Rationale: Offloading concurrency tracking to Redis removes heavy transactional lock contention from PostgreSQL, ensuring that throughput remains predictable during order spikes.
3: 
  Outbox events are ingested into ClickHouse for operational telemetry using the following optimized schema:
      CREATE TABLE order_transactions.routed_events
(
    order_id String,
    vendor_id UInt64,
    customer_lat Float64,
    customer_lon Float64,
    original_total Float64,
    discounted_total Float64,
    routed_at DateTime,
    routing_reason LowCardinality(String)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(routed_at)
ORDER BY (vendor_id, routed_at);

Partition Key: Partitioning by toYYYYMM(routed_at) optimizes cold-data retention and speeds up time-series queries
Sorting Key: Ordering by (vendor_id, routed_at) enables rapid columnar index scans for vendor-specific performance reports and billing reconciliation



4:
   To handle a 10x increase in concurrent traffic, the current reliance on live PostgreSQL spatial intersection queries (ST_Contains) must be replaced
       The Bottleneck: Evaluating complex coordinate-in-polygon math dynamically across thousands of active geo-vectors saturates database CPU cores and introduces heavy query locking.
       The Solution: Migrate to an In-Memory Spatial Index (Uber H3 Hexagonal Grid). Vendor polygons are pre-allocated into static 64-bit integer H3 spatial rings at boot time. Incoming customer coordinates are indexed to an H3 cell directly within the Go application layer using standard math libraries. This shifts the calculation from an expensive disk/CPU geometry search down to a simple $O(1)$ memory hash-map lookup, uncoupling database performance from traffic growth