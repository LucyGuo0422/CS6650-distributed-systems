from locust import FastHttpUser, task, between
import random
import json


# ---------- Test with FastHttpUser ----------
class ProductFastHttpUser(FastHttpUser):
    wait_time = between(1, 3)

    @task(8)
    def get_product(self):
        product_id = random.randint(1, 100)
        self.client.get(f"/products/{product_id}", name="/products/[id]")

    @task(2)
    def add_product(self):
        product_id = random.randint(1, 100)
        payload = {
            "product_id": product_id,
            "sku": f"SKU-{product_id:04d}",
            "manufacturer": f"Manufacturer-{random.randint(1, 20)}",
            "category_id": random.randint(1, 50),
            "weight": random.randint(100, 5000),
            "some_other_id": random.randint(1, 1000),
        }
        self.client.post(
            f"/products/{product_id}/details",
            json=payload,
            name="/products/[id]/details",
        )