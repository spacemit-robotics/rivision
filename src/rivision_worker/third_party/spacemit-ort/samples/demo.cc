// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#include <assert.h>
#include <onnxruntime_cxx_api.h>
#include <onnxruntime_session_options_config_keys.h>
#include <sys/syscall.h>
#include <unistd.h>

#include <thread>

#include "imagenet_labels.h"
#include "spacemit_ort_env.h"
#include "utils.h"

//static int SetSchedAffinity(const std::vector<int>& cpu_ids) {
//  cpu_set_t cpuset;
//  CPU_ZERO(&cpuset);
//  pthread_t main_thread = pthread_self();
//  for (size_t i = 0; i < cpu_ids.size(); i++) {
//    CPU_SET(cpu_ids[i], &cpuset);
//  }
//  int s = pthread_setaffinity_np(main_thread, sizeof(cpu_set_t), &cpuset);
//  if (s != 0) {
//    fprintf(stderr, "set thread affinity error.");
//  }
//  return 0;
//}

int main(int argc, char ** argv) {
    if (argc < 3) {
        fprintf(stderr,
                "Usage: %s [net_name=str] [net_param_path=str] [profile_prefix=str] \
            [img_file_path=str] [num_threads=int] [loop_count=int] [mean_value=str] [std_value=str] [dyn_shape=str]\n",
                argv[0]);
        return -1;
    }

    const char * net_name       = argv[1];
    const char * net_param_path = argv[2];
    char *       img_file_path  = nullptr;
    char *       profile_prefix = nullptr;
    char *       mean_str       = nullptr;
    char *       std_str        = nullptr;
    char *       dyn_shape_str  = nullptr;
    int          num_threads    = 1;
    int          loop_count     = 1;

    if (argc >= 4) {
        profile_prefix = argv[3];
    }

    if (argc >= 5) {
        img_file_path = argv[4];
    }

    if (argc >= 6) {
        num_threads = atoi(argv[5]);
    }

    if (argc >= 7) {
        loop_count = atoi(argv[6]);
    }

    if (argc >= 8) {
        mean_str = argv[7];
    }

    if (argc >= 9) {
        std_str = argv[8];
    }

    if (argc >= 10) {
        dyn_shape_str = argv[9];
    }

    std::cout << "run demo [" << net_name << "], num_threads=" << num_threads << std::endl;

    Ort::Env                                     env(ORT_LOGGING_LEVEL_WARNING, "ort_test");
    Ort::AllocatorWithDefaultOptions             allocator;
    Ort::SessionOptions                          session_options;
    std::unordered_map<std::string, std::string> provider_options;

    session_options.SetIntraOpNumThreads(1);
    session_options.SetInterOpNumThreads(1);

    provider_options["SPACEMIT_EP_INTRA_THREAD_NUM"] = std::to_string(num_threads);

    OrtStatus * status = Ort::SessionOptionsSpaceMITEnvInit(session_options, provider_options);

    if (profile_prefix != nullptr && strcmp(profile_prefix, "None") != 0) {
        std::string profile_path = net_name;
        profile_path             = profile_prefix + profile_path;
        session_options.EnableProfiling(profile_path.c_str());

        std::string opt_net_path = net_name;
        opt_net_path             = profile_prefix + opt_net_path + "_opt.onnx";
        session_options.SetOptimizedModelFilePath(opt_net_path.c_str());
    }

    auto       time_start = std::chrono::high_resolution_clock::now();
    NetSession session(env, net_param_path, session_options);
    auto       session_init_duration =
        std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::high_resolution_clock::now() - time_start)
            .count();

    // imagenet preprocess
    std::vector<float> mean_value  = { 123.675f, 116.28f, 103.53f };
    std::vector<float> scale_value = { 58.395f, 57.12f, 57.375f };

    if (mean_str != nullptr && std_str != nullptr && strcmp(mean_str, "None") != 0 && strcmp(std_str, "None") != 0) {
        std::cout << "parse mean and std " << mean_str << ", " << std_str << std::endl;
        ParseMeanStd(mean_value, scale_value, mean_str, std_str);
    }

    std::vector<std::vector<int64_t>> dyn_input_shape_info;
    if (dyn_shape_str != nullptr) {
        ParseDynShape(dyn_input_shape_info, dyn_shape_str);
    }

    auto input_count  = session.GetInputCount();
    auto output_count = session.GetOutputCount();

    std::vector<Ort::Value> input_values;
    input_values.reserve(input_count);
    for (size_t i = 0; i < input_count; i++) {
        if (i < dyn_input_shape_info.size()) {
            session.SetInputShape(i, dyn_input_shape_info[i]);
        } else {
            auto input_shape = session.GetInputShape(i);
            // set dyn shape
            for (size_t si = 0; si < input_shape.size(); si++) {
                if (input_shape[si] <= 0) {
                    input_shape[si] = 1;
                }
            }
            session.SetInputShape(i, input_shape);
        }
        input_values.push_back(session.CreatorInputValue(i));
    }

    if (input_count == 1 && input_values[0].GetTensorTypeAndShapeInfo().GetElementType() ==
                                ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT) {
        if (img_file_path != nullptr && strcmp(img_file_path, "None") != 0) {
            auto input_shape = session.GetInputShape(0);
            if (input_shape.size() == 4) {
                std::cout << "Read img file from " << img_file_path << std::endl;
                float * input_img_data = input_values[0].GetTensorMutableData<float>();
                ReadImageRawFile<float>(img_file_path, input_img_data, input_shape[0], input_shape[1],
                                        input_shape[2] * input_shape[3], mean_value, scale_value);
            }
        }
    }

    // warm up and predict
    std::vector<float> time_duration_vec;
    time_start          = std::chrono::high_resolution_clock::now();
    auto output_tensors = session.Run(input_values.data());
    auto inference_duration =
        std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::high_resolution_clock::now() - time_start)
            .count();
    std::cout << "init time cost: " << session_init_duration / 1000.0 << " ms." << std::endl;
    std::cout << "inference time cost: " << inference_duration / 1000.0 << " ms." << std::endl;

    if (output_tensors.size() == 1) {
        auto predict_vec = session.ImageNetPostProcess(output_tensors[0]);

        if (predict_vec.size() == 1000) {
            auto pred_index = predict_vec[0];
            auto label_name = IMAGENET_LABEL_LIST[pred_index];
            std::cout << "predict object is " << label_name << "." << std::endl;
        } else if (predict_vec.size() == 1001) {
            auto pred_index = predict_vec[0];
            auto label_name = pred_index >= 1 ? IMAGENET_LABEL_LIST[pred_index - 1] : "background";
            std::cout << "predict object is " << label_name << "." << std::endl;
        }
    }

    for (size_t i = 0; i < loop_count; i++) {
        time_start         = std::chrono::high_resolution_clock::now();
        output_tensors     = session.Run(input_values.data());
        inference_duration = std::chrono::duration_cast<std::chrono::microseconds>(
                                 std::chrono::high_resolution_clock::now() - time_start)
                                 .count();
        time_duration_vec.push_back(inference_duration / 1000.0);

        if (i % 10 == 0) {
            std::cout << "inference time cost: " << time_duration_vec[time_duration_vec.size() - 1] << " ms."
                      << std::endl;
        }
    }

    if (loop_count >= 5) {
        std::sort(time_duration_vec.begin(), time_duration_vec.end());

        float time_avg = 0.0f;
        for (size_t i = 1; i < time_duration_vec.size() - 1; i++) {
            time_avg += time_duration_vec[i];
        }
        time_avg /= (time_duration_vec.size() - 2);
        std::cout << "inference time cost avg: " << time_avg << " ms. min: " << time_duration_vec[0]
                  << " ms. max: " << time_duration_vec[time_duration_vec.size() - 1] << " ms." << std::endl;
    }

    return 0;
}
