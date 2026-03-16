from locust import HttpUser, task, between

SAMPLE_ORDER = {
    "customer_id": 1,
    "items": [
        {"name": "Widget", "quantity": 2, "price": 9.99},
    ],
}


class SyncUser(HttpUser):
    """Phase 1/2: hits /orders/sync only. Use for baseline bottleneck analysis."""
    wait_time = between(0.5, 1.5)

    @task
    def post_sync_order(self):
        self.client.post("/orders/sync", json=SAMPLE_ORDER)


class AsyncUser(HttpUser):
    """Phase 3/4/5: hits /orders/async only. Use for queue buildup and scaling experiments."""
    wait_time = between(0.5, 1.5)

    @task
    def post_async_order(self):
        self.client.post("/orders/async", json=SAMPLE_ORDER)


class MixedUser(HttpUser):
    """Hits both endpoints with equal weight."""
    wait_time = between(0.5, 1.5)

    @task
    def post_sync_order(self):
        self.client.post("/orders/sync", json=SAMPLE_ORDER)

    @task
    def post_async_order(self):
        self.client.post("/orders/async", json=SAMPLE_ORDER)
