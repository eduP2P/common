import json
import os
import sys
from pathlib import Path

# Exit codes
EXIT_PERFORMANCE_SIMILAR = 0
EXIT_COMPARISON_FAILED = 1
EXIT_PERFORMANCE_WORSE = 1
EXIT_PERFORMANCE_BETTER = 0

# Ensure both parameters are provided
if len(sys.argv) - 1 != 2:
    print(f"""
Usage: python {sys.argv[0]} <BASELINE PERFORMANCE TEST DATA> <NEW PERFORMANCE TEST DATA>

The two parameters should be either system test logs containing only performance tests, or extracted performance-test-data artifacts from GitHub Actions

The output of this script is formatted as markdown such that it can be used in GitHub job step summaries""")
    exit(EXIT_COMPARISON_FAILED)

baseline=sys.argv[1]
new=sys.argv[2]

# This dictionary defines which measurements to compare, and when to consider two measurements worse/better/similar
COMPARISON_CONFIG = {
    "Target bitrate": { 
        "packet_loss": {
            # Second condition to prevent performance being considered better when new = 0 and baseline is very small
            "better": lambda new, baseline: new < 0.9 * baseline and new < baseline - 1,
            # Second condition to prevent performance being considered worse when baseline = 0 and new is very small
            "worse": lambda new, baseline: new > 1.1 * baseline or (baseline == 0 and new > baseline + 1)
        }
    }
}

# Keep track of whether performance is worse or better
performance_worse = False
performance_better = False

def failure(reason: str):
    print("# ❌ Performance comparison failed")
    print(reason)
    exit(EXIT_COMPARISON_FAILED)

def check_same_performance_test(new_data: dict, baseline_data: dict, rel_path: str):
    """Check whether the two data files contain the same performance test, otherwise comparison is not possible"""

    same_test_var = new_data["test_var"] == baseline_data["test_var"]

    if not same_test_var:
        failure(f"mismatch in test variable or its values for {rel_path}")

    baseline_metrics = set(baseline_data["measurements"].keys())
    new_metrics = set(new_data["measurements"].keys())
    all_required_metrics = baseline_metrics <= new_metrics

    if not all_required_metrics:
        failure(f"for {rel_path}, some of the measurements in the baseline data are not present in the new data")

def report_performance_change(better: bool, metric: str, idx: int, baseline: float, new: float):
    change = "improved" if better else "degraded"
    print(f"- {metric} {change} at index {idx}: {baseline:.1f} -> {new:.1f}")
    
def compare_measurements(new_data: dict, baseline_data: dict):
    test_var = baseline_data["test_var"]["label"] 

    if not(test_var in COMPARISON_CONFIG.keys()):
        return
    
    # Keep track of performance difference by modifying the global variables
    global performance_worse, performance_better
    
    metrics_to_compare = COMPARISON_CONFIG[test_var].keys()
    new_measurements = new_data["measurements"]
    baseline_measurements = baseline_data["measurements"]

    for metric in metrics_to_compare:
        metric_label = baseline_measurements[metric]["label"]
        new_values = new_measurements[metric]["values"]["average"]["eduP2P"]
        baseline_values = baseline_measurements[metric]["values"]["average"]["eduP2P"]
        is_worse = COMPARISON_CONFIG[test_var][metric]["worse"]
        is_better = COMPARISON_CONFIG[test_var][metric]["better"]
        
        for i, (new_val, baseline_val) in enumerate(zip(new_values, baseline_values)):
            if is_worse(new_val, baseline_val):
                report_performance_change(False, metric_label, i, baseline_val, new_val)
                performance_worse = True

            if is_better(new_val, baseline_val):
                report_performance_change(True, metric_label, i, baseline_val, new_val)
                performance_better = True and not performance_worse # Worse performance has higher priority than better performance

        

# Iterate over all data files from baseline performance test data
cwd = os.getcwd()
baseline_files = Path(f"{cwd}/{baseline}").rglob("performance_test_data.json*")
print("# Comparison details")

for path in baseline_files:
    path = str(path)

    # Get relative path by removing current working directory + baseline directory prefix
    rel_path = path[len(cwd) + len(baseline) + 1:]
    print(f"### Comparing {rel_path}...")

    # Attempt to open same performance test file in new data
    try:
        with open(f"{cwd}/{new}/{rel_path}") as f_new:
            new_data = json.load(f_new)
    except FileNotFoundError:
        failure(f"{rel_path} is present in {baseline}, but not in {new}")
        
    with open(path) as f_baseline:
        baseline_data = json.load(f_baseline)

    check_same_performance_test(new_data, baseline_data, rel_path)
    compare_measurements(new_data, baseline_data)

# Print final conclusion about performance
if performance_worse:
    print(f"# 📉 Total performance has degraded")
    exit(EXIT_PERFORMANCE_WORSE)
elif performance_better:
    print(f"# 📈 Total performance has improved!")
    exit(EXIT_PERFORMANCE_BETTER)

print(f"# ✅ No significant performance change")
exit(EXIT_PERFORMANCE_SIMILAR)

