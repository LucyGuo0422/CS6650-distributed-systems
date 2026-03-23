import json

with open("results/mysql_test_results.json") as f:
    mysql = json.load(f)
for r in mysql:
    r["database"] = "mysql"

with open("results/dynamodb_test_results.json") as f:
    dynamo = json.load(f)
for r in dynamo:
    r["database"] = "dynamodb"

combined = mysql + dynamo
with open("results/combined_results.json", "w") as f:
    json.dump(combined, f, indent=2)
print(f"Combined: {len(combined)} records")
