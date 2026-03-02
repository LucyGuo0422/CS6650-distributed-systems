from locust import HttpUser, task, between
import random


class BrokenOrderUser(HttpUser):
    wait_time = between(0.1, 0.5)

    @task
    def place_order(self):
        payload = {
            "item_id": random.choice([1, 2]),
            "quantity": random.randint(1, 5),
        }
        with self.client.post("/order", json=payload, catch_response=True) as resp:
            if resp.status_code == 200:
                resp.success()
            else:
                resp.failure(f"status={resp.status_code} body={resp.text[:80]}")


class FixedOrderUser(HttpUser):
    wait_time = between(0.1, 0.5)

    @task(10)
    def place_order(self):
        payload = {
            "item_id": random.choice([1, 2]),
            "quantity": random.randint(1, 5),
        }
        with self.client.post("/order", json=payload, catch_response=True) as resp:
            if resp.status_code == 200:
                resp.success()
            elif resp.status_code == 503:
                # Circuit open — expected under outage, not a failure
                resp.success()
            else:
                resp.failure(f"status={resp.status_code} body={resp.text[:80]}")

    @task(3)
    def invalid_order(self):
        payload = random.choice([
            {"item_id": -1, "quantity": 1},
            {"item_id": 1, "quantity": 0},
            {"item_id": 1, "quantity": 9999},
            {},
        ])
        with self.client.post("/order", json=payload, catch_response=True) as resp:
            if resp.status_code == 400:
                resp.success()
            else:
                resp.failure(f"Expected 400, got {resp.status_code}")

    @task(1)
    def check_health(self):
        self.client.get("/healthz")
