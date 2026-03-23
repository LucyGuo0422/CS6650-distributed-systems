import requests, json, time, datetime, sys, os

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
RESULTS_FILE = os.path.join(SCRIPT_DIR, "..", "results", "dynamodb_test_results.json")

BASE_URL = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
results = []

def record(operation, response, start):
    results.append({
        "operation": operation,
        "response_time": round((time.time() - start) * 1000, 2),
        "success": response.status_code in (200, 201, 204),
        "status_code": response.status_code,
        "timestamp": datetime.datetime.now(datetime.UTC).isoformat()
    })

cart_ids = []

print("Creating 50 carts...")
for i in range(50):
    start = time.time()
    r = requests.post(f"{BASE_URL}/shopping-carts",
                      json={"customer_id": i + 1})
    record("create_cart", r, start)
    if r.status_code == 201:
        cart_ids.append(r.json()["shopping_cart_id"])

print("Adding items to 50 carts...")
for i, cart_id in enumerate(cart_ids[:50]):
    start = time.time()
    r = requests.post(f"{BASE_URL}/shopping-carts/{cart_id}/items",
                      json={"product_id": (i % 10) + 1, "quantity": 1})
    record("add_items", r, start)

print("Retrieving 50 carts...")
for cart_id in cart_ids[:50]:
    start = time.time()
    r = requests.get(f"{BASE_URL}/shopping-carts/{cart_id}")
    record("get_cart", r, start)

with open(RESULTS_FILE, "w") as f:
    json.dump(results, f, indent=2)

successes = sum(1 for r in results if r["success"])
print(f"\nDone! {successes}/150 successful")
print(f"Results saved to {RESULTS_FILE}")
