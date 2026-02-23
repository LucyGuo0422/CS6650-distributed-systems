from locust import task, between
from locust.contrib.fasthttp import FastHttpUser
import random

# Common search terms that will find results
SEARCH_TERMS = [
    "alpha", "beta", "gamma", "delta",
    "electronics", "books", "home", "sports",
    "product", "omega", "nova", "zeta"
]

class ProductSearchUser(FastHttpUser):
    # Minimal wait to maximize pressure on the server
    wait_time = between(0.1, 0.5)

    @task
    def search_products(self):
        term = random.choice(SEARCH_TERMS)
        with self.client.get(
            f"/products/search?q={term}",
            catch_response=True
        ) as response:
            if response.status_code == 200:
                response.success()
            else:
                response.failure(f"Got status {response.status_code}")