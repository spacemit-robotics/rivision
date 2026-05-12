// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#include <assert.h>

#include <algorithm>
#include <array>
#include <chrono>
#include <condition_variable>
#include <cstdlib>
#include <exception>
#include <filesystem>
#include <iostream>
#include <mutex>
#include <thread>

#include "spacemit_ort_env.h"
#include "utils.h"

using ImageNetLabel = std::pair<std::string, int>;

struct AccuracyStats {
    size_t samples{ 0 };
    size_t top1_hits{ 0 };
    size_t top5_hits{ 0 };

    void Add(bool top1_ok, bool top5_ok) {
        ++samples;
        top1_hits += top1_ok ? 1 : 0;
        top5_hits += top5_ok ? 1 : 0;
    }

    double Top1Accuracy() const {
        return samples == 0 ? 0.0 : static_cast<double>(top1_hits) * 100.0 / static_cast<double>(samples);
    }

    double Top5Accuracy() const {
        return samples == 0 ? 0.0 : static_cast<double>(top5_hits) * 100.0 / static_cast<double>(samples);
    }
};

struct SampleRunResult {
    std::vector<Ort::Value> outputs;
    double                  cost_ms{ 0.0 };
    std::exception_ptr      error;
};

struct WorkerContext {
    std::mutex              mutex;
    std::condition_variable cv;
    std::thread             thread;
    ImageNetLabel           label;
    SampleRunResult         result;
    bool                    has_task{ false };
    bool                    completed{ false };
    bool                    stop{ false };
};

std::vector<ImageNetLabel> ReadImageNetLabels(const std::string & file_path) {
    std::ifstream ifs(file_path);
    if (!ifs) {
        throw std::runtime_error("open file failed");
    }
    std::string                line;
    std::vector<ImageNetLabel> labels;
    std::filesystem::path      img_list_path(file_path);
    std::string                img_directory = img_list_path.parent_path().string();
    while (std::getline(ifs, line)) {
        if (!line.empty()) {
            auto label_line = StringSplit(line, ',');
            if (label_line.size() == 2) {
                labels.push_back(std::make_pair(img_directory + "/" + label_line[0], std::stoi(label_line[1])));
            }
        }
    }
    return labels;
}

std::pair<bool, bool> EvaluatePrediction(NetSession & session, Ort::Value & output_value, int label_idx) {
    auto predict_vec = session.ImageNetPostProcess(output_value);
    if (predict_vec.empty()) {
        throw std::runtime_error("predict result is empty");
    }

    const int expected_label = predict_vec.size() == 1001 ? label_idx + 1 : label_idx;
    bool      top1_ok        = predict_vec[0] == expected_label;
    bool      top5_ok        = false;

    const size_t topk = std::min<size_t>(5, predict_vec.size());
    for (size_t i = 0; i < topk; ++i) {
        if (predict_vec[i] == expected_label) {
            top5_ok = true;
            break;
        }
    }

    return { top1_ok, top5_ok };
}

void PrintAccuracyProgress(const std::string & prefix, const AccuracyStats & stats) {
    std::cout << prefix << ", samples=" << stats.samples << ", top1_accuracy=" << stats.Top1Accuracy()
              << ", top5_accuracy=" << stats.Top5Accuracy() << std::endl;
}

int main(int argc, char ** argv) {
    if (argc < 5) {
        fprintf(stderr,
                "Usage: %s [net_name=str] [net_param_path=str] [img_file_list_path=str] [num_threads=int] "
                "[mean_value=str] [std_value=str] [inter_thread_num=int]\n",
                argv[0]);
        return -1;
    }

    Ort::Env     env(ORT_LOGGING_LEVEL_WARNING, "ort_test");
    const char * net_name               = argv[1];
    const char * net_param_path         = argv[2];
    const char * img_file_list_path     = argv[3];
    char *       mean_str               = nullptr;
    char *       std_str                = nullptr;
    const int    num_threads            = atoi(argv[4]);
    int          inter_thread_num       = 1;
    const char * inter_thread_env_cstr  = std::getenv("SPACEMIT_EP_INTER_THREAD_NUM");

    if (inter_thread_env_cstr != nullptr) {
        inter_thread_num = std::atoi(inter_thread_env_cstr);
    }

    if (argc >= 6) {
        mean_str = argv[5];
    }

    if (argc >= 7) {
        std_str = argv[6];
    }

    if (argc >= 8) {
        inter_thread_num = std::atoi(argv[7]);
    }

    std::cout << "imagenet_test [" << net_name << "], num_threads=" << num_threads
              << ", inter_thread_num=" << inter_thread_num << std::endl;

    auto labels = ReadImageNetLabels(img_file_list_path);
    if (labels.empty()) {
        throw std::runtime_error("imagenet label list is empty");
    }

    std::cout << "Load ImageNet test labels " << labels.size() << std::endl;

    Ort::SessionOptions                          session_options;
    std::unordered_map<std::string, std::string> provider_options;

    session_options.SetIntraOpNumThreads(1);
    session_options.SetInterOpNumThreads(1);
    provider_options["SPACEMIT_EP_INTRA_THREAD_NUM"] = std::to_string(num_threads);
    provider_options["SPACEMIT_EP_INTER_THREAD_NUM"] = std::to_string(inter_thread_num);
    Ort::ThrowOnError(Ort::SessionOptionsSpaceMITEnvInit(session_options, provider_options));

    NetSession session(env, net_param_path, session_options);

    std::vector<float> mean_value  = { 123.675f, 116.28f, 103.53f };
    std::vector<float> scale_value = { 58.395f, 57.12f, 57.375f };

    if (mean_str != nullptr && std_str != nullptr) {
        std::cout << "parse mean and std " << mean_str << ", " << std_str << std::endl;
        ParseMeanStd(mean_value, scale_value, mean_str, std_str);
    }

    auto input_count  = session.GetInputCount();
    auto output_count = session.GetOutputCount();

    if (input_count != 1 || (output_count != 1 && output_count != 2)) {
        throw std::runtime_error("input or output count mismatch");
    }

    for (size_t i = 0; i < input_count; i++) {
        auto input_shape = session.GetInputShape(i);
        for (size_t si = 0; si < input_shape.size(); si++) {
            if (input_shape[si] <= 0) {
                input_shape[si] = 1;
            }
        }
        session.SetInputShape(i, input_shape);
    }

    auto input_shape = session.GetInputShape(0);

    auto create_input_values = [&](const std::string & file_path) {
        std::vector<Ort::Value> input_values;
        input_values.reserve(input_count);
        for (size_t i = 0; i < input_count; i++) {
            input_values.push_back(session.CreatorInputValue(i));
        }

        float * input_img_data = input_values[0].GetTensorMutableData<float>();
        ReadImageRawFile<float>(file_path, input_img_data, input_shape[0], input_shape[1], input_shape[2] * input_shape[3],
                                mean_value, scale_value);
        return input_values;
    };

    auto run_one_sample = [&](const ImageNetLabel & label, SampleRunResult & result) {
        try {
            auto inputs = create_input_values(label.first);
            auto start  = std::chrono::high_resolution_clock::now();
            result.outputs =
                session.Run(inputs.data());
            result.cost_ms = std::chrono::duration_cast<std::chrono::microseconds>(
                                 std::chrono::high_resolution_clock::now() - start)
                                 .count() /
                             1000.0;
        } catch (...) {
            result.error = std::current_exception();
        }
    };

    auto apply_result = [&](SampleRunResult & result, const ImageNetLabel & label, AccuracyStats & stats) {
        if (result.error) {
            std::rethrow_exception(result.error);
        }
        if (result.outputs.size() != output_count) {
            throw std::runtime_error("output count mismatch");
        }

        auto [top1_ok, top5_ok] = EvaluatePrediction(session, result.outputs[output_count - 1], label.second);
        stats.Add(top1_ok, top5_ok);
    };

    std::vector<double> time_duration_vec;

    if (inter_thread_num > 1 && labels.size() >= 2) {
        std::cout << "multi executor dataset validation enabled, inter_thread_num=" << inter_thread_num << std::endl;

        auto worker_loop = [&](WorkerContext & worker) {
            while (true) {
                ImageNetLabel current_label;
                {
                    std::unique_lock<std::mutex> lock(worker.mutex);
                    worker.cv.wait(lock, [&]() { return worker.has_task || worker.stop; });
                    if (worker.stop) {
                        return;
                    }

                    current_label     = worker.label;
                    worker.has_task   = false;
                    worker.completed  = false;
                    worker.result     = SampleRunResult{};
                }

                run_one_sample(current_label, worker.result);

                {
                    std::lock_guard<std::mutex> lock(worker.mutex);
                    worker.completed = true;
                }
                worker.cv.notify_one();
            }
        };

        auto submit_task = [&](WorkerContext & worker, const ImageNetLabel & label) {
            std::lock_guard<std::mutex> lock(worker.mutex);
            worker.label     = label;
            worker.result    = SampleRunResult{};
            worker.completed = false;
            worker.has_task  = true;
            worker.cv.notify_one();
        };

        auto wait_task = [&](WorkerContext & worker) {
            std::unique_lock<std::mutex> lock(worker.mutex);
            worker.cv.wait(lock, [&]() { return worker.completed; });
        };

        auto stop_worker = [&](WorkerContext & worker) {
            {
                std::lock_guard<std::mutex> lock(worker.mutex);
                worker.stop = true;
            }
            worker.cv.notify_one();
        };

        std::array<WorkerContext, 2> workers;
        workers[0].thread = std::thread([&]() { worker_loop(workers[0]); });
        workers[1].thread = std::thread([&]() { worker_loop(workers[1]); });

        auto shutdown_workers = [&]() {
            for (auto & worker : workers) {
                stop_worker(worker);
            }
            for (auto & worker : workers) {
                if (worker.thread.joinable()) {
                    worker.thread.join();
                }
            }
        };

        SampleRunResult warmup_result0;
        SampleRunResult warmup_result1;
        AccuracyStats    thread0_stats;
        AccuracyStats    thread1_stats;
        AccuracyStats    overall_stats;

        try {
            submit_task(workers[0], labels[0]);
            submit_task(workers[1], labels[1]);
            wait_task(workers[0]);
            wait_task(workers[1]);

            warmup_result0 = std::move(workers[0].result);
            warmup_result1 = std::move(workers[1].result);
            if (warmup_result0.error) {
                std::rethrow_exception(warmup_result0.error);
            }
            if (warmup_result1.error) {
                std::rethrow_exception(warmup_result1.error);
            }
            std::cout << "multi executor warmup finished: thread0=" << warmup_result0.cost_ms
                      << " ms, thread1=" << warmup_result1.cost_ms
                      << " ms, max=" << std::max(warmup_result0.cost_ms, warmup_result1.cost_ms) << " ms." << std::endl;

            for (size_t label_index = 0; label_index < labels.size(); label_index += 2) {
                if (label_index + 1 < labels.size()) {
                    const auto label0 = labels[label_index];
                    const auto label1 = labels[label_index + 1];

                    submit_task(workers[0], label0);
                    submit_task(workers[1], label1);
                    wait_task(workers[0]);
                    wait_task(workers[1]);

                    auto thread0_result = std::move(workers[0].result);
                    auto thread1_result = std::move(workers[1].result);

                    apply_result(thread0_result, label0, thread0_stats);
                    apply_result(thread1_result, label1, thread1_stats);
                    apply_result(thread0_result, label0, overall_stats);
                    apply_result(thread1_result, label1, overall_stats);

                    time_duration_vec.push_back(std::max(thread0_result.cost_ms, thread1_result.cost_ms));

                    if (overall_stats.samples % 100 == 0) {
                        std::cout << "concurrent validation progress: samples=" << overall_stats.samples
                                  << ", thread0_cost=" << thread0_result.cost_ms << " ms"
                                  << ", thread1_cost=" << thread1_result.cost_ms << " ms" << std::endl;
                        PrintAccuracyProgress("thread0", thread0_stats);
                        PrintAccuracyProgress("thread1", thread1_stats);
                        PrintAccuracyProgress("overall", overall_stats);
                    }
                } else {
                    submit_task(workers[0], labels[label_index]);
                    wait_task(workers[0]);

                    auto thread0_result = std::move(workers[0].result);
                    apply_result(thread0_result, labels[label_index], thread0_stats);
                    apply_result(thread0_result, labels[label_index], overall_stats);
                    time_duration_vec.push_back(thread0_result.cost_ms);
                }
            }

            PrintAccuracyProgress("thread0", thread0_stats);
            PrintAccuracyProgress("thread1", thread1_stats);
            PrintAccuracyProgress("overall", overall_stats);
            std::cout << net_name << ", multi_executor overall top1_accuracy: " << overall_stats.Top1Accuracy()
                      << ", top5_accuracy: " << overall_stats.Top5Accuracy() << std::endl;
        } catch (...) {
            shutdown_workers();
            throw;
        }

        shutdown_workers();
    } else {
        AccuracyStats single_thread_stats;
        if (inter_thread_num > 1 && labels.size() < 2) {
            std::cout << "multi executor requested but dataset has fewer than 2 samples, fallback to single-thread validation"
                      << std::endl;
        }

        for (const auto & label : labels) {
            SampleRunResult result;
            run_one_sample(label, result);
            apply_result(result, label, single_thread_stats);
            time_duration_vec.push_back(result.cost_ms);

            if (single_thread_stats.samples % 100 == 0) {
                std::cout << "test_idx: " << single_thread_stats.samples
                          << ", inference_time_cost=" << result.cost_ms << " ms" << std::endl;
                PrintAccuracyProgress("single_thread", single_thread_stats);
            }
        }

        std::cout << net_name << ", top1_accuracy: " << single_thread_stats.Top1Accuracy()
                  << ", top5_accuracy: " << single_thread_stats.Top5Accuracy() << std::endl;
    }

    if (time_duration_vec.size() >= 5) {
        std::sort(time_duration_vec.begin(), time_duration_vec.end());

        double time_avg = 0.0;
        for (size_t i = 1; i < time_duration_vec.size() - 1; i++) {
            time_avg += time_duration_vec[i];
        }
        time_avg /= static_cast<double>(time_duration_vec.size() - 2);
        std::cout << "inference time cost avg: " << time_avg << " ms. min: " << time_duration_vec.front()
                  << " ms. max: " << time_duration_vec.back() << " ms." << std::endl;
    }

    return 0;
}
