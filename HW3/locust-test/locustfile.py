from locust import FastHttpUser, task, between
import random
import time

class AlbumAPITestUser(FastHttpUser):
    """
    Load test for:
      GET  /albums
      GET  /albums/:id
      POST /albums
    """
    wait_time = between(0.1, 0.5)

    # Use existing seeded IDs so GET-by-id never 404s.
    seeded_ids = ["1", "2", "3"]

    @task(2)
    def get_albums(self):
        # Name= groups stats nicely in UI
        with self.client.get("/albums", name="GET /albums", catch_response=True) as r:
            if r.status_code != 200:
                r.failure(f"Expected 200, got {r.status_code}")

    @task(1)
    def get_album_by_id(self):
        album_id = random.choice(self.seeded_ids)
        with self.client.get(f"/albums/{album_id}", name="GET /albums/:id", catch_response=True) as r:
            if r.status_code != 200:
                r.failure(f"Expected 200, got {r.status_code}")

    @task(1)
    def post_album(self):
        # Use a time-based unique ID so you don't collide if you later add checks.
        new_id = str(int(time.time() * 1000)) + str(random.randint(0, 999))

        payload = {
            "id": new_id,
            "title": f"Load Test Album {new_id}",
            "artist": "Locust",
            "price": round(random.uniform(10.0, 60.0), 2)
        }

        headers = {"Content-Type": "application/json"}

        with self.client.post("/albums", json=payload, headers=headers, name="POST /albums", catch_response=True) as r:
            if r.status_code != 201:
                r.failure(f"Expected 201, got {r.status_code}")
