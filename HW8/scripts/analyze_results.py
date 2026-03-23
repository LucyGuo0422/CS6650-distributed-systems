import json, statistics

with open("results/combined_results.json") as f:
    data = json.load(f)

for db in ["mysql", "dynamodb"]:
    for op in ["create_cart", "add_items", "get_cart"]:
        times = [r["response_time"] for r in data
                 if r["database"] == db and r["operation"] == op and r["success"]]
        times.sort()
        n = len(times)
        print(f"{db} | {op}")
        print(f"  avg={statistics.mean(times):.1f}ms  "
              f"p50={times[n//2]:.1f}ms  "
              f"p95={times[int(n*0.95)]:.1f}ms  "
              f"p99={times[int(n*0.99)]:.1f}ms")
