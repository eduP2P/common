import json
import numpy as np
from pathlib import Path
import re
import sys
import matplotlib.pyplot as plt

if len(sys.argv) != 2:
    print(f"""
Usage: python {sys.argv[0]} <LOG DIRECTORY>

Recursively iterates over every json file in the specified directory, and creates graphs based on their data""")
    exit(1)

LOG_DIR = sys.argv[1]

TEST_VARS = {
    "bitrate" : {
        "label" : "Target bitrate",
        "unit" : "Mbps"
    },
    "packet_loss" : {
        "label" : "Simulated packet loss",
        "unit" : "%"
    }
}

def extract_data(data : dict, extracted_data : dict) -> dict:
    data = data["end"]["sum"]
    
    for metric in extracted_data.keys():
        metric_key = extracted_data[metric]["json_key"]
        measurement = float(data[metric_key])

        # Apply metric's transform function if there is one
        try:
            transform = extracted_data[metric]["transform"]
            measurement = transform(measurement)
        except KeyError:
            pass

        extracted_data[metric]["values"].append(measurement)

    return extracted_data

def create_graph(test_var : str, test_var_values : list[float], metric : str, extracted_data : dict, save_path : str):
    metric_data = extracted_data[metric]
    y = metric_data["values"]

    test_var_label = TEST_VARS[test_var]["label"]
    test_var_unit = TEST_VARS[test_var]["unit"]
    metric_label = metric_data["label"]
    metric_unit = metric_data["unit"]

    plt.plot(test_var_values, y)
    plt.xlabel(f"{test_var_label} ({test_var_unit})")
    plt.ylabel(f"{metric_label} ({metric_unit})")
    plt.title(f"{metric_label} for varying {test_var_label}")
    plt.ticklabel_format(useOffset=False)
    plt.savefig(f"{save_path}/performance_test_{metric}.png")
    plt.clf()

def file_iteration(test_dir : str, test_var : str) -> tuple[list[float], dict]:
    test_var_values = []
    extracted_data = {
        "bitrate" : {
            "label" : "Measured bitrate",
            "json_key" : "bits_per_second",
            "unit" : "Mbps",
            "values" : [],
            "transform" : lambda x: x/10**6
        },
        "jitter" : {
            "label" : "Jitter",
            "json_key" : "jitter_ms",
            "unit" : "ms",
            "values" : []
        },
        "packet_loss" : {
            "label" : "Measured packet loss",
            "json_key" : "lost_percent",
            "unit" : "%",
            "values" : []
        }
    }

    paths = Path(test_dir).glob(f"{test_var}=*")

    for path in paths:
        path_str = str(path)
        test_file = path_str.split('/')[-1]

        # Capture test variable value
        p = re.compile(f"{test_var}=(.*).json")
        m = p.match(test_file)
        test_var_val = float(m.group(1))

        test_var_values.append(test_var_val)

        with open(path_str, 'r') as file:
            data = json.load(file)
            extracted_data = extract_data(data, extracted_data)

    # Sort data
    sorted_indices=np.argsort(test_var_values)
    test_var_values = np.array(test_var_values)[sorted_indices]

    for metric in extracted_data.keys():
        extracted_data[metric]["values"] = np.array(extracted_data[metric]["values"])[sorted_indices]

    return test_var_values, extracted_data

def maybe_plural(amount, noun):
    if amount > 1:
        return noun + 's'
    else:
        return noun
    
def test_iteration():
    paths = Path(LOG_DIR).rglob("performance_tests_*")
    n_tests = 0

    for path in paths:
        n_tests += 1

        test_path = str(path)
        test_dir = test_path.split('/')[-1]
        parent_path = '/'.join(test_path.split('/')[:-1])

        # Capture test variable
        p = re.compile("performance_tests_(.*)")
        m = p.match(test_dir)
        test_var = m.group(1)
        
        test_var_values, extracted_data = file_iteration(test_path, test_var)

        for metric in extracted_data.keys():
            create_graph(test_var, test_var_values, metric, extracted_data, parent_path)

    if n_tests > 0:
        print(f"Generated graphs to visualize {n_tests} {maybe_plural(n_tests, 'performance test')}")

test_iteration()