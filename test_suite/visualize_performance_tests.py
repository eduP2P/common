import json
import numpy as np
from pathlib import Path
import re
import sys
import matplotlib.pyplot as plt
from itertools import groupby

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
    },
    "delay" : {
        "label" : "Simulated one-way delay",
        "unit" : "ms"
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
        
        # Extract test variable values and corresponding measurements
        test_var_values, extracted_data = repetition_iteration(test_path, test_var)
        extracted_data = aggregate_repetitions(extracted_data)

        # Create dictionary containing test variable info
        test_var_dict = TEST_VARS[test_var]
        test_var_dict["values"] = test_var_values

        with open(f"{parent_path}/performance_test_data.json", 'w') as file:
            # Delete transform key from bitrate metric, since it is not JSON serializable
            del extracted_data["bitrate"]["transform"]

            # Delete json_key key from all metrics, since they are no longer needed
            for k in extracted_data.keys():
                del extracted_data[k]["json_key"]

            # Merge test variable info and measurements into one dictionary
            data_dict = {
                "test_var": test_var_dict,
                "measurements": extracted_data
            }

            json.dump(data_dict, file, indent=4)

        for metric in extracted_data.keys():
            create_performance_graph(test_var, test_var_values, metric, extracted_data, parent_path)

        create_variance_grid(data_dict, parent_path)

    if n_tests > 0:
        plural = "s" if n_tests > 1 else ""
        print(f"Generated graphs to visualize {n_tests} performance test{plural}")

# Recursively iterate over all repetition<i> subdirectories
def repetition_iteration(test_path: str, test_var: str) -> tuple[list[float], dict]:
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
            "label" : "Measured HTTP latency",
            "json_key" : "delay_ms",
            "unit" : "ms",
            "values" : {}
        },
    }

    # Delay is not affected by the iperf3 target bitrate, so this data has not been measured
    if(test_var == "bitrate"):
        del extracted_data["delay"]

    paths = Path(test_path).rglob("repetition*")
    
    # Iterate over repetitions sorted from lowest to highest number (default sorting order is inconsistent)
    for path in sorted(paths, key=lambda p: str(p)):
        repetition_path = str(path)
        repetition_id = repetition_path.split('/')[-1]

        # Initialize the dictionary of measurements for this repetition
        for metric in extracted_data.keys():
            extracted_data[metric]["values"][repetition_id] = {}

        test_var_values, extracted_data = connection_iteration(repetition_path, repetition_id, test_var, extracted_data)

    return test_var_values, extracted_data

# Recursively iterate over all connection subdirectories (eduP2P/WireGuard/Direct)
def connection_iteration(repetition_path: str, repetition_id: str, test_var: str, extracted_data: dict) -> tuple[list[float], dict]:
    paths = Path(repetition_path).glob("*")

    for path in paths:
        connection_path = str(path)
        connection_type = connection_path.split('/')[-1]

        # Initialize the lists of measurements for this connection type
        for metric in extracted_data.keys():
            extracted_data[metric]["values"][repetition_id][connection_type] = []
        
        test_var_values, extracted_data = file_iteration(connection_type, connection_path, repetition_id, test_var, extracted_data)

    return test_var_values, extracted_data

# Recursively iterate over all json files in the connection subdirectories (each file corresponds to one test variable value)
def file_iteration(connection_type: str, connection_path: str, repetition_id: str, test_var: str, extracted_data: dict) -> tuple[list[float], dict]:
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
            extracted_data = extract_data(connection_type, repetition_id, data, extracted_data)

    # Sort data
    sorted_indices=np.argsort(test_var_values)
    test_var_values = list(np.array(test_var_values)[sorted_indices])

    for metric in extracted_data.keys():
        sorted_measurements = np.array(extracted_data[metric]["values"][repetition_id][connection_type])[sorted_indices]
        extracted_data[metric]["values"][repetition_id][connection_type] = list(sorted_measurements)

    return test_var_values, extracted_data

def extract_data(connection_type: str, repetition_id: str, data: dict, extracted_data: dict) -> dict:
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

        extracted_data[metric]["values"][repetition_id][connection_type].append(measurement)

    return extracted_data

def aggregate_repetitions(extracted_data: dict) -> dict:
    for metric in extracted_data.keys():
        measurements = extracted_data[metric]["values"]
        connection_dicts = [v for _, v in measurements.items()]
        connection_measurement_pairs = [(k, v) for c in connection_dicts for k, v in list(c.items())] 
        aggregated_measurements = {}

        # Group the measurements by connection type to compute the average
        for connection_type, groups in groupby(sorted(connection_measurement_pairs, reverse=True), key=lambda t: t[0]):
            aggregated_measurements[connection_type] = list(np.array([group[1] for group in groups]).mean(axis=0))

        measurements["average"] = aggregated_measurements

    return extracted_data

# Given a dictionary containing the label and unit of a metric, returns a string to describe the metric on a graph axis
def axis_label(label_unit_dict: dict) -> str:
    return f"{label_unit_dict["label"]} ({label_unit_dict["unit"]})"

# Graph to illustrate the performance of eduP2P, possibly by comparing against WireGuard and/or a direct connection
def create_performance_graph(test_var: str, test_var_values: list[float], metric: str, extracted_data: dict, save_path: str):
    metric_data = extracted_data[metric]
    connection_measurements = metric_data["values"]["average"]
    test_var_label = TEST_VARS[test_var]["label"]
    metric_label = metric_data["label"]

    # Different line styles in case they overlap
    line_styles=["-", "--", ":"]
    line_widths=[4,3,2]

    for i, connection in enumerate(connection_measurements.keys()):
        x = test_var_values
        y = connection_measurements[connection]   
        ls=line_styles[i]  
        lw=line_widths[i]
        plt.plot(x, y, linestyle=ls, linewidth=lw, label=connection)

    plt.xlabel(axis_label(TEST_VARS[test_var]))
    plt.ylabel(axis_label(metric_data))
    plt.title(f"{metric_label} for varying {test_var_label}")
    plt.ticklabel_format(useOffset=False)
    plt.legend()
    plt.tight_layout()
    plt.savefig(f"{save_path}/performance_test_{metric}.png")
    plt.clf()

# Create an <n_metrics> * <n_connections> grid of plots showing the variance in measurements across repetitions for each metric and connection type
def create_variance_grid(data_dict: dict, save_path: str):
    test_var_info = data_dict["test_var"]
    test_var_values = test_var_info["values"]

    measurements = data_dict["measurements"]    
    metrics = list(measurements.keys())
    n_metrics = len(metrics)
    reps_and_avg = measurements[metrics[0]]["values"]

    # This indicates that reps_and_avg = ["repetition1", "average"], so only 1 repetition is performed 
    if len(reps_and_avg) == 2:
        return
    
    connections = list(reps_and_avg["average"].keys())
    n_connections = len(connections)
    fig, ax = plt.subplots(n_metrics, n_connections)

    # Iterate over rows in the grid
    for i, metric in enumerate(metrics):
        metric_dict = measurements[metric]
        create_variance_col(ax[i], i, test_var_values, metric_dict["values"]) 

        # Y label is the same for each row, so we only set it on the first column to save space 
        ax[i][0].set_ylabel(axis_label(metric_dict))

    # X label is the same for each subplot, so we only set it on the bottom row to save space
    for j in range(n_connections): 
        ax[n_metrics-1][j].set_xlabel(axis_label(test_var_info))

    # Display legend shared between the subplots and save the figure
    handles, labels = ax[0][0].get_legend_handles_labels()
    fig.legend(handles, labels, loc='upper center', bbox_to_anchor=(0.5, 1.03), ncol=len(reps_and_avg)//2)

    # Place suptitle higher to free up space for legend
    fig.suptitle("Variance of measurements over multiple repetitions", y=1.06)

    subplot_size = 4
    fig.set_figheight(n_metrics * subplot_size)
    fig.set_figwidth(n_connections * subplot_size)
    fig.tight_layout()
    fig.savefig(f"{save_path}/performance_test_variance.png", bbox_inches="tight") # bbox_inches prevents suptitle and legend from being cropped

# Fill one column of the variance grid with graphs
def create_variance_col(ax: np.ndarray[plt.Axes], i: int, test_var_values: list[float], measurements: dict):
    for k, repetition_dict in measurements.items():
        for j, conn in enumerate(repetition_dict.keys()):
            conn_measurements = repetition_dict[conn]

            # Make line representing the average stand out from lines representing individual repetitions
            if k == "average":
                ax[j].plot(test_var_values, conn_measurements, label=k, linestyle="-", linewidth=3, color="black")
            else:
                ax[j].plot(test_var_values, conn_measurements, label=k, linestyle="--", linewidth=1.5)

            # Each connection type takes up a separate column, so put the connection type above the top subplots 
            if(i == 0):
                ax[j].set_title(conn)

            # On the Y axis, use scientific notation for numbers outside the range [1e-3, 1e4] to prevent them from crossing into other subplots
            ax[j].ticklabel_format(axis='y', style='sci', scilimits=(-3,4))

test_iteration()