import onnx
import onnxruntime as ort
import numpy as np
import os
import argparse
import tempfile
import shutil
from onnx import numpy_helper

tensor_type_to_np_type = {
    "tensor(float)": "float32",
    "tensor(int8)": "int8",
    "tensor(uint8)": "uint8",
    "tensor(int16)": "int16",
    "tensor(uint16)": "uint16",
    "tensor(int32)": "int32",
    "tensor(uint32)": "uint32",
    "tensor(int64)": "int64",
    "tensor(uint64)": "uint64",
}

feed_dict = {}

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-m", "--model_path", type=str, required=True, help="Path to the model"
    )
    parser.add_argument(
        "-o", "--output_dir", type=str, required=True, help="Path to the output dir"
    )
    parser.add_argument(
        "-n", "--num", type=int, required=True, help="Number of test cases"
    )
    parser.add_argument(
         "--min", type=float, required=False, default=0.0
    )
    parser.add_argument(
         "--max", type=float, required=False, default=1.0
    )
    parser.add_argument(
         "--img_list_file", type=str, required=False, default=None
    )
    parser.add_argument(
         "--mean", type=str, required=False, default="123.675,116.28,103.53"
    )
    parser.add_argument(
         "--std", type=str, required=False, default="58.395,57.12,57.375"
    )
    args = parser.parse_args()

    session = ort.InferenceSession(args.model_path)

    output_names = [o.name for o in session.get_outputs()]

    if os.path.exists(args.output_dir):
        shutil.rmtree(args.output_dir)
    os.makedirs(args.output_dir)

    shutil.copy2(args.model_path, args.output_dir)

    img_list = []
    mean_value = np.array([float(i) for i in args.mean.split(",")]).astype(np.float32).reshape(-1, 1, 1)
    std_value = np.array([float(i) for i in args.std.split(",")]).astype(np.float32).reshape(-1, 1, 1)
    if os.path.isfile(args.img_list_file):
        with open(args.img_list_file, "r") as f:
            lines = f.readlines()
            for line in lines:
                line = line.strip()
                if len(line) == 0:
                    continue
                base_file_name = line.split(",")[0].replace("bin", "npy").strip()
                img_list.append(os.path.join(
                    os.path.dirname(args.img_list_file),
                    base_file_name
                ))

    for i in range(args.num):
        test_case_dir = os.path.join(args.output_dir, "test_case_{}".format(i))
        os.makedirs(test_case_dir)

        input_idx = 0
        for in_var in session.get_inputs():
            shape = in_var.shape
            dtype = tensor_type_to_np_type.get(in_var.type)

            if i < len(img_list):
                print("loading image {}".format(img_list[i]))
                img_data = np.load(img_list[i]).astype(np.float32)
                img_data = (img_data - mean_value) / std_value
                feed_dict[in_var.name] = img_data
            else:
                if dtype in {"int64", "int32", "uint64", "uint32"}:
                    feed_dict[in_var.name] = np.zeros(shape, dtype)
                else:
                    random_value = np.random.random(shape).astype(dtype)
                    random_value = random_value * (args.max - args.min) + args.min
                    feed_dict[in_var.name] = random_value

            feed_tensor_pb = numpy_helper.from_array(feed_dict[in_var.name], in_var.name)

            with open("{}/input_{}.pb".format(test_case_dir, input_idx), "wb") as f:
                f.write(feed_tensor_pb.SerializeToString())
            input_idx += 1

        outputs = session.run(output_names, feed_dict)

        output_idx = 0
        for output_tensor, output_name in zip(outputs, output_names):
            feed_tensor_pb = numpy_helper.from_array(output_tensor, output_name)
            with open("{}/output_{}.pb".format(test_case_dir, output_idx), "wb") as f:
                f.write(feed_tensor_pb.SerializeToString())
            output_idx += 1