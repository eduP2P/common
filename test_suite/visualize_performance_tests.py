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

# Independent test variables that can be configured in the performance tests
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

# Recursively iterate over all performance_tests_<variable> directories
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
        
        test_var_values, extracted_data = connection_iteration(test_path, test_var)

        for metric in extracted_data.keys():
            create_graph(test_var, test_var_values, metric, extracted_data, parent_path)

    if n_tests > 0:
        plural = "s" if n_tests > 1 else ""
        print(f"Generated graphs to visualize {n_tests} performance test{plural}")

# Recursively iterate over all connection subdirectories (eduP2P/WireGuard/Direct)
def connection_iteration(test_path : str, test_var : str) -> dict:
    extracted_data = {
        "bitrate" : {
            "label" : "Measured bitrate",
            "json_key" : "bits_per_second",
            "unit" : "Mbps",
            "values" : {},
            "transform" : lambda x: x/10**6
        },
        "jitter" : {
            "label" : "Jitter",
            "json_key" : "jitter_ms",
            "unit" : "ms",
            "values" : {}
        },
        "packet_loss" : {
            "label" : "Measured packet loss",
            "json_key" : "lost_percent",
            "unit" : "%",
            "values" : {}
        },
        "delay" : {
            "label" : "Delay",
            "json_key" : "delay_ms",
            "unit" : "ms",
            "values" : {}
        },
    }

    paths = Path(test_path).glob("*")

    for path in paths:
        connection_path = str(path)
        connection_type = connection_path.split('/')[-1]

        # Initialize the lists of measurements for this connection type
        for metric in extracted_data.keys():
            extracted_data[metric]["values"][connection_type] = []
        
        test_var_values, extracted_data = file_iteration(connection_type, connection_path, test_var, extracted_data)

    return test_var_values, extracted_data

# Recursively iterate over all json files in the connection subdirectories (each file corresponds to one test variable value)
def file_iteration(connection_type : str, connection_path : str, test_var : str, extracted_data : dict) -> tuple[list[float], dict]:
    test_var_values = []
    paths = Path(connection_path).glob(f"{test_var}=*")

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
            extracted_data = extract_data(connection_type, data, extracted_data)

    # Sort data
    sorted_indices=np.argsort(test_var_values)
    test_var_values = np.array(test_var_values)[sorted_indices]

    for metric in extracted_data.keys():
        extracted_data[metric]["values"][connection_type] = np.array(extracted_data[metric]["values"][connection_type])[sorted_indices]

    return test_var_values, extracted_data

def extract_data(connection_type : str, data : dict, extracted_data : dict) -> dict:
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

        extracted_data[metric]["values"][connection_type].append(measurement)

    return extracted_data

def create_graph(test_var : str, test_var_values : list[float], metric : str, extracted_data : dict, save_path : str):
    metric_data = extracted_data[metric]
    connection_measurements = metric_data["values"]

    test_var_label = TEST_VARS[test_var]["label"]
    test_var_unit = TEST_VARS[test_var]["unit"]
    metric_label = metric_data["label"]
    metric_unit = metric_data["unit"]

    # Different line styles in case they overlap
    line_styles=["-", "--", ":"]
    line_widths=[4,3,2]

    for i, connection in enumerate(connection_measurements.keys()):
        y = connection_measurements[connection]   
        ls=line_styles[i]  
        lw=line_widths[i]

        # Plot the measured independent variable values on the X axis instead of the target values, unless the measured values are already plotted on the Y axis
        if metric == test_var:
            plt.plot(test_var_values, y, linestyle=ls, linewidth=lw, label=connection)
            x_label = test_var_label
        else:
            measured_test_var_values = sorted(extracted_data[test_var]["values"][connection])
            plt.plot(measured_test_var_values, y, linestyle=ls, linewidth=lw, label=connection)
            x_label = extracted_data[test_var]["label"]

    plt.xlabel(f"{x_label} ({test_var_unit})")
    plt.ylabel(f"{metric_label} ({metric_unit})")
    plt.title(f"{metric_label} for varying {x_label}")
    plt.ticklabel_format(useOffset=False)
    plt.legend()
    
    plt.savefig(f"{save_path}/performance_test_{metric}.png")
    plt.clf()

test_iteration()