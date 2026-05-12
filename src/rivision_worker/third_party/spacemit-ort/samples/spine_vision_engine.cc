// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#include "spine_vision_engine.h"
#include "onnxruntime_session_options_config_keys.h"
#include <assert.h>
#include <cctype>
#include <dlfcn.h>
#include <fstream>
#include <iostream>

namespace onnxruntime {
extern const OrtApi * g_ort;

namespace spacemit {

namespace {
void SaveFloatTensorToBin(const std::string & filename, const float * data, size_t num_elements) {
    std::ofstream file(filename, std::ios::binary);
    if (!file.is_open()) {
        throw std::runtime_error("Cannot open file for writing: " + filename);
    }
    file.write(reinterpret_cast<const char *>(data), num_elements * sizeof(float));
    file.close();
}

bool IsQwen3VLArchitecture(const std::string & architecture) {
    return architecture == "Qwen3VL";
}

OrtStatus * InitSpaceMITExecutionProvider(Ort::SessionOptions &                              options,
                                          const std::unordered_map<std::string, std::string> provider_options) {
    auto                      num_entries = provider_options.size();
    std::vector<const char *> keys, values;
    if (num_entries > 0) {
        keys.reserve(num_entries);
        values.reserve(num_entries);

        for (const auto & entry : provider_options) {
            keys.push_back(entry.first.c_str());
            values.push_back(entry.second.c_str());
        }
    }
    void * handle = dlopen("libspacemit_ep.so", RTLD_NOW);
    if (!handle) {
        throw std::runtime_error(std::string("Failed to load libspacemit_ep.so: ") + dlerror());
    }
    OrtStatus * (*ep_init)(OrtSessionOptions *, const char * const *, const char * const *, size_t);
    ep_init = reinterpret_cast<decltype(ep_init)>(dlsym(handle, "OrtSessionOptionsSpaceMITEnvInit"));
    if (!ep_init) {
        throw std::runtime_error(std::string("Failed to find OrtSessionOptionsSpaceMITEnvInit: ") + dlerror());
    }
    auto sts = ep_init(options, keys.data(), values.data(), num_entries);
    return sts;
}
}  // namespace

Ort::Value & SpineVisionModelEngine::SetInputTensor(std::string & input_binary_path) {
    auto type_info   = vision_session_.GetInputTypeInfo(0);
    auto tensor_info = type_info.GetTensorTypeAndShapeInfo();

    ONNXTensorElementDataType input_type  = tensor_info.GetElementType();
    std::vector<int64_t>      input_shape = tensor_info.GetShape();
    size_t                    input_size  = 1;
    for (size_t i = 0; i < input_shape.size(); i++) {
        input_size *= input_shape[i];
    }

    vision_binary_data_.resize(input_size);
    std::ifstream file(input_binary_path.c_str(), std::ios::binary);

    if (!file.is_open()) {
        std::cerr << "error: failed to open input.bin" << std::endl;
    }

    size_t expected_file_size = input_size * sizeof(float);

    file.seekg(0, std::ios::end);
    size_t actual_file_size = file.tellg();
    file.seekg(0, std::ios::beg);

    if (actual_file_size != expected_file_size) {
        std::cerr << "error: file size mismatch" << std::endl;
        std::cerr << "expected size: " << expected_file_size << std::endl;
        std::cerr << "actual size: " << actual_file_size << std::endl;
        file.close();
    }

    file.read(reinterpret_cast<char *>(vision_binary_data_.data()), actual_file_size);

    if (!file) {
        std::cerr << "error: failed to read input.bin" << std::endl;
    }
    file.close();

    Ort::MemoryInfo memory_info =
        Ort::MemoryInfo::CreateCpu(OrtAllocatorType::OrtArenaAllocator, OrtMemType::OrtMemTypeDefault);

    input_tensor_ = Ort::Value::CreateTensor<float>(memory_info, vision_binary_data_.data(), vision_binary_data_.size(),
                                                    input_shape.data(), input_shape.size());
    std::ofstream ofile(input_binary_path.c_str(), std::ios::binary);
    ofile.write(reinterpret_cast<const char *>(vision_binary_data_.data()), vision_binary_data_.size() * sizeof(float));
    ofile.close();

    return input_tensor_;
}

void SpineVisionModelEngine::InitSesstionOptions() {
    OrtLoggingLevel logging_level = run_config_.f_verbose ? ORT_LOGGING_LEVEL_VERBOSE : ORT_LOGGING_LEVEL_WARNING;
    spine_llm_env_                = Ort::Env(logging_level, "Default");

    if (run_config_.enable_cpu_mem_arena) {
        session_options_.EnableCpuMemArena();
    } else {
        session_options_.DisableCpuMemArena();
    }
    if (run_config_.enable_memory_pattern && run_config_.execution_mode == ExecutionMode::ORT_SEQUENTIAL) {
        session_options_.EnableMemPattern();
    } else {
        session_options_.DisableMemPattern();
    }
    session_options_.SetExecutionMode(run_config_.execution_mode);

    // Set any extra session configuration entries provided by the user via command-line arguments.
    //
    // Some session config entries can also be set via dedicated command-line options.
    // If the user uses multiple command-line options to set the same session config entry,
    // we'll print a warning. Note that the dedicated command-line options will take precedence.
    const auto & user_session_configs = run_config_.session_config_entries;
    for (auto & it : user_session_configs) {
        session_options_.AddConfigEntry(it.first.c_str(), it.second.c_str());
    }

    auto warn_dup_config_entry = [&user_session_configs](const char * key) -> void {
        if (user_session_configs.find(key) != user_session_configs.end()) {
            fprintf(stderr, "[WARNING]: Trying to set session config entry '%s' via multiple command-line options\n",
                    key);
        }
    };

    if (run_config_.disable_spinning) {
        warn_dup_config_entry(kOrtSessionOptionsConfigAllowIntraOpSpinning);
        fprintf(stdout, "Disabling intra-op thread spinning entirely\n");
        session_options_.AddConfigEntry(kOrtSessionOptionsConfigAllowIntraOpSpinning, "0");
    }

    if (run_config_.disable_spinning_between_run) {
        warn_dup_config_entry(kOrtSessionOptionsConfigForceSpinningStop);
        fprintf(stdout, "Disabling intra-op thread spinning between runs\n");
        session_options_.AddConfigEntry(kOrtSessionOptionsConfigForceSpinningStop, "1");
    }

    if (!run_config_.register_custom_op_path.empty()) {
        session_options_.RegisterCustomOpsLibrary(run_config_.register_custom_op_path.c_str());
    }

    // Set optimization level.
    session_options_.SetGraphOptimizationLevel(run_config_.optimization_level);
    if (!run_config_.profile_file.empty()) {
        session_options_.EnableProfiling(run_config_.profile_file.c_str());
    }
    if (!run_config_.optimized_model_path.empty()) {
        session_options_.SetOptimizedModelFilePath(run_config_.optimized_model_path.c_str());
    }
    if (run_config_.set_denormal_as_zero) {
        warn_dup_config_entry(kOrtSessionOptionsConfigSetDenormalAsZero);
        session_options_.AddConfigEntry(kOrtSessionOptionsConfigSetDenormalAsZero, "1");
    }
    if (!run_config_.free_dim_name_overrides.empty()) {
        for (const auto & dim_override : run_config_.free_dim_name_overrides) {
            if (g_ort->AddFreeDimensionOverrideByName(session_options_, dim_override.first.c_str(),
                                                      dim_override.second) != nullptr) {
                fprintf(stderr, "AddFreeDimensionOverrideByName failed for named dimension: %s\n",
                        dim_override.first.c_str());
            } else {
                fprintf(stdout, "Overriding dimension with name, %s, to %d\n", dim_override.first.c_str(),
                        (int) dim_override.second);
            }
        }
    }
    if (!run_config_.free_dim_denotation_overrides.empty()) {
        for (const auto & dim_override : run_config_.free_dim_denotation_overrides) {
            if (g_ort->AddFreeDimensionOverride(session_options_, dim_override.first.c_str(), dim_override.second) !=
                nullptr) {
                fprintf(stderr, "AddFreeDimensionOverride failed for dimension denotation: %s\n",
                        dim_override.first.c_str());
            } else {
                fprintf(stdout, "Overriding dimension with denotation, %s, to %d\n", dim_override.first.c_str(),
                        (int) dim_override.second);
            }
        }
    }
}

Ort::Session & SpineVisionModelEngine::CreateVisionModelSession() {
    std::unordered_map<std::string, std::string> spacemit_ep_provider_options = ep_config_;
    InitSesstionOptions();
    InitSpaceMITExecutionProvider(session_options_, spacemit_ep_provider_options);
    vision_session_     = Ort::Session(spine_llm_env_, model_file_path_.c_str(), session_options_);
    size_t output_count = vision_session_.GetOutputCount();
    output_names_.resize(output_count);
    Ort::AllocatorWithDefaultOptions a;
    for (size_t i = 0; i != output_count; ++i) {
        auto output_name = vision_session_.GetOutputNameAllocated(i, a);
        assert(output_name != nullptr);
        output_names_[i] = output_name.get();
    }
    output_names_raw_ptr_.resize(output_count);
    for (size_t i = 0; i != output_count; ++i) {
        output_names_raw_ptr_[i] = output_names_[i].c_str();
    }
    const size_t input_count = static_cast<size_t>(vision_session_.GetInputCount());
    input_names_str_.resize(input_count);
    input_names_.resize(input_count);
    for (size_t i = 0; i != input_count; ++i) {
        auto input_name = vision_session_.GetInputNameAllocated(i, a);
        assert(input_name != nullptr);
        input_names_str_[i] = input_name.get();
        input_names_[i]     = input_names_str_[i].c_str();
    }

    return vision_session_;
}

std::vector<float> SpineVisionModelEngine::RunSession(Ort::Value & input_tensor) {
    std::vector<Ort::Value> output_tensors =
        vision_session_.Run(Ort::RunOptions{ nullptr }, input_names_.data(), &input_tensor, input_names_.size(),
                            output_names_raw_ptr_.data(), output_names_raw_ptr_.size());

    if (IsQwen3VLArchitecture(architecture_) && output_tensors.size() > 1) {
        struct OutputView {
            const float * data       = nullptr;
            int64_t       n_tokens   = 0;
            int64_t       n_embd     = 0;
            size_t        n_elements = 0;
        };

        std::vector<OutputView> outputs;
        outputs.reserve(output_tensors.size());

        for (size_t output_idx = 0; output_idx < output_tensors.size(); ++output_idx) {
            Ort::Value & output = output_tensors[output_idx];

            auto                 tensor_info = output.GetTensorTypeAndShapeInfo();
            std::vector<int64_t> shape       = tensor_info.GetShape();

            if (tensor_info.GetElementType() != ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT) {
                throw std::runtime_error("Expected float32 output from Qwen3VL vision model");
            }

            if (shape.size() == 3 && shape[0] == 1) {
                shape = { shape[1], shape[2] };
            }
            if (shape.size() != 2) {
                throw std::runtime_error("Unexpected output shape from Qwen3VL vision encoder");
            }

            const int64_t n_tokens = shape[0];
            const int64_t n_embd   = shape[1];
            if (n_tokens <= 0 || n_embd <= 0) {
                throw std::runtime_error("Invalid output shape from Qwen3VL vision encoder");
            }

            const size_t n_elements = tensor_info.GetElementCount();
            if (n_elements != (size_t) n_tokens * (size_t) n_embd) {
                throw std::runtime_error("Qwen3VL vision output element count does not match shape");
            }

            outputs.push_back({ output.GetTensorData<float>(), n_tokens, n_embd, n_elements });
        }

        const int64_t n_tokens = outputs[0].n_tokens;
        const int64_t n_embd   = outputs[0].n_embd;
        for (size_t output_idx = 1; output_idx < outputs.size(); ++output_idx) {
            if (outputs[output_idx].n_tokens != n_tokens || outputs[output_idx].n_embd != n_embd) {
                throw std::runtime_error(
                    "Qwen3VL vision multi-output tensors must have identical [tokens, hidden] shape");
            }
        }

        const size_t       n_outputs      = outputs.size();
        const size_t       out_n_embd     = (size_t) n_embd * n_outputs;
        const size_t       total_elements = (size_t) n_tokens * out_n_embd;
        std::vector<float> result(total_elements);

        for (int64_t token = 0; token < n_tokens; ++token) {
            float * dst = result.data() + (size_t) token * out_n_embd;
            for (size_t output_idx = 0; output_idx < n_outputs; ++output_idx) {
                const OutputView & output = outputs[output_idx];
                const float *      src    = output.data + (size_t) token * (size_t) n_embd;
                std::copy(src, src + (size_t) n_embd, dst + output_idx * (size_t) n_embd);
            }
        }

        std::cerr << "[SMT][vision] Qwen3VL concatenated " << n_outputs << " outputs [" << n_tokens << ", " << n_embd
                  << "] -> [" << n_tokens << ", " << out_n_embd << "]\n";

        return result;
    }

    Ort::Value & output = output_tensors[0];

    auto                      tensor_info = output.GetTensorTypeAndShapeInfo();
    ONNXTensorElementDataType type        = tensor_info.GetElementType();
    std::vector<int64_t>      shape       = tensor_info.GetShape();

    size_t num_elements = tensor_info.GetElementCount();

    if (shape.size() == 3 && shape[0] == 1) {
        shape = { shape[1], shape[2] };
    } else if (shape.size() != 2) {
        printf("Unexpected output shape from vision encoder!");
    }

    int64_t n_tokens       = shape[0];
    int64_t n_embd         = shape[1];
    size_t  total_elements = n_tokens * n_embd;

    if (tensor_info.GetElementType() != ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT) {
        printf("Expected float32 output from vision model!");
    }
    const float * data = output.GetTensorData<float>();

    return std::vector<float>(data, data + total_elements);
}

}  // namespace spacemit
}  // namespace onnxruntime
