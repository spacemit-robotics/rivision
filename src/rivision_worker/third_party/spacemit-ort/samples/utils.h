// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#pragma once

#include <onnxruntime_cxx_api.h>

#include <algorithm>
#include <cstring>
#include <fstream>
#include <iostream>
#include <string>
#include <vector>

std::vector<std::string> StringSplit(const std::string & s, char delimiter) {
    std::vector<std::string> result;
    size_t                   start = 0;
    size_t                   end   = s.find(delimiter);
    while (end != std::string::npos) {
        result.push_back(s.substr(start, end - start));
        start = end + 1;
        end   = s.find(delimiter, start);
    }
    result.push_back(s.substr(start));
    return result;
}

void ParseMeanStd(std::vector<float> & mean_value,
                  std::vector<float> & std_value,
                  std::string          mean_str,
                  std::string          std_str) {
    auto mean_str_vec = StringSplit(mean_str, ',');
    auto std_str_vec  = StringSplit(std_str, ',');

    mean_value.clear();
    std_value.clear();

    for (size_t i = 0; i < mean_str_vec.size(); i++) {
        mean_value.push_back(atof(mean_str_vec[0].c_str()));
    }
    for (size_t i = 0; i < std_str_vec.size(); i++) {
        std_value.push_back(atof(std_str_vec[0].c_str()));
    }
}

void ParseDynShape(std::vector<std::vector<int64_t>> & dyn_shape, std::string dyn_shape_str) {
    auto single_shape_str_list = StringSplit(dyn_shape_str, ';');
    dyn_shape.clear();

    for (auto single_shape_str : single_shape_str_list) {
        std::cout << "parse dyn shape " << single_shape_str << std::endl;
        auto                 shape_str = StringSplit(single_shape_str, ',');
        std::vector<int64_t> shape;
        for (auto s : shape_str) {
            shape.push_back(atoi(s.c_str()));
        }
        dyn_shape.push_back(shape);
    }
}

template <typename T>
void ReadImageRawFile(const std::string &      file_path,
                      T *                      input_tensor,
                      int                      batch_size,
                      int                      channel_num,
                      int                      channel_stride,
                      const std::vector<float> mean_value,
                      const std::vector<float> scale_value) {
    std::ifstream ifs(file_path, std::ios::binary);
    if (!ifs) {
        fprintf(stderr, "open file failed\n");
        return;
        // throw std::runtime_error("open file failed");
    }
    char magic[6];
    ifs.read(magic, 6);
    size_t fileSize = 0;
    if (std::string(magic, 6) == "\x93NUMPY") {
        ifs.seekg(0, std::ios::end);
        fileSize = ifs.tellg() - 128;
        ifs.seekg(128, std::ios::beg);
    } else {
        ifs.seekg(0, std::ios::end);
        fileSize = ifs.tellg();
        ifs.seekg(0, std::ios::beg);
    }

    if (fileSize != batch_size * channel_num * channel_stride ||
        channel_num * channel_stride != mean_value.size() * channel_stride) {
        throw std::runtime_error("file size mismatch");
    }

    char img_data = 0;
    for (size_t i = 0; i < fileSize; i++) {
        size_t c_idx = i / channel_stride;
        ifs.read(&img_data, 1);
        input_tensor[i] = (((float) img_data) - mean_value[c_idx]) / scale_value[c_idx];
    }

    ifs.close();
}

class NetSession {
  public:
    explicit NetSession(const Ort::Env & env, const ORTCHAR_T * model_path, const Ort::SessionOptions & options) {
        session_ = std::make_unique<Ort::Session>(env, model_path, options);
        GetSessionInputOutput();
    }

    Ort::Value CreatorInputValue(size_t index) {
        if (index >= input_shape_info_.size()) {
            throw std::runtime_error("input index error");
        }
        auto   shape_dims        = input_shape_info_[index];
        auto   element_type      = input_dtype_info_[index];
        size_t input_tensor_size = input_tensor_size_info_[index];
        if (input_tensor_size == 0) {
            throw std::runtime_error("input size error, you should SetInputShape first.");
        }
        if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT) {
            auto input_tensor =
                Ort::Value::CreateTensor<float>(session_allocator_, shape_dims.data(), shape_dims.size());
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_INT8) {
            auto input_tensor =
                Ort::Value::CreateTensor<int8_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_UINT8) {
            auto input_tensor =
                Ort::Value::CreateTensor<uint8_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT16) {
            auto input_tensor =
                Ort::Value::CreateTensor<Ort::Float16_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_INT16) {
            auto input_tensor =
                Ort::Value::CreateTensor<int16_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            auto * input_data = input_tensor.GetTensorMutableRawData();
            auto   input_size = input_tensor.GetTensorTypeAndShapeInfo().GetElementCount();
            memset(input_data, 0, input_size * sizeof(int16_t));
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_UINT16) {
            auto input_tensor =
                Ort::Value::CreateTensor<uint16_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            auto * input_data = input_tensor.GetTensorMutableRawData();
            auto   input_size = input_tensor.GetTensorTypeAndShapeInfo().GetElementCount();
            memset(input_data, 0, input_size * sizeof(uint16_t));
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_INT32) {
            auto input_tensor =
                Ort::Value::CreateTensor<int32_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            auto * input_data = input_tensor.GetTensorMutableRawData();
            auto   input_size = input_tensor.GetTensorTypeAndShapeInfo().GetElementCount();
            memset(input_data, 0, input_size * sizeof(int32_t));
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_UINT32) {
            auto input_tensor =
                Ort::Value::CreateTensor<uint32_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            auto * input_data = input_tensor.GetTensorMutableRawData();
            auto   input_size = input_tensor.GetTensorTypeAndShapeInfo().GetElementCount();
            memset(input_data, 0, input_size * sizeof(uint32_t));
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_INT64) {
            auto input_tensor =
                Ort::Value::CreateTensor<int64_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            auto * input_data = input_tensor.GetTensorMutableRawData();
            auto   input_size = input_tensor.GetTensorTypeAndShapeInfo().GetElementCount();
            memset(input_data, 0, input_size * sizeof(int64_t));
            return input_tensor;
        } else if (element_type == ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_UINT64) {
            auto input_tensor =
                Ort::Value::CreateTensor<uint64_t>(session_allocator_, shape_dims.data(), shape_dims.size());
            return input_tensor;
        } else {
            throw std::runtime_error("not implemented dtype");
        }
        return Ort::Value(nullptr);
    }

    std::vector<Ort::Value> Run(const Ort::Value * input_values) {
        auto output_tensors =
            session_->Run(Ort::RunOptions{ nullptr }, input_node_names_.data(), input_values, input_node_names_.size(),
                          output_node_names_.data(), output_node_names_.size());
        return output_tensors;
    }

    std::vector<int> ImageNetPostProcess(Ort::Value & output_value) {
        std::vector<int> predict_indx;
        if (output_value.GetTensorTypeAndShapeInfo().GetElementType() ==
            ONNXTensorElementDataType::ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT) {
            auto   output_shape = output_value.GetTensorTypeAndShapeInfo().GetShape();
            size_t numel        = 1;
            for (size_t i = 0; i < output_shape.size(); i++) {
                numel *= output_shape[i];
            }

            const float *                      output_data_ptr = output_value.GetTensorData<float>();
            std::vector<std::pair<int, float>> output_data_vec;
            for (size_t i = 0; i < numel; i++) {
                output_data_vec.push_back(std::make_pair(i, output_data_ptr[i]));
            }

            std::sort(output_data_vec.begin(), output_data_vec.end(),
                      [](std::pair<int, float> lhs, std::pair<int, float> rhs) { return lhs.second > rhs.second; });
            for (auto pred : output_data_vec) {
                predict_indx.push_back(pred.first);
            }
        }

        return predict_indx;
    }

    const size_t GetInputCount() { return input_node_names_.size(); }

    const size_t GetOutputCount() { return output_node_names_.size(); }

    const std::vector<int64_t> GetInputShape(size_t index) {
        if (index >= input_shape_info_.size()) {
            throw std::runtime_error("input index error");
        }
        return input_shape_info_[index];
    }

    void SetInputShape(size_t index, std::vector<int64_t> input_shape) {
        if (index >= input_shape_info_.size()) {
            throw std::runtime_error("input index error");
        }
        input_tensor_size_info_[index] = 1;
        input_shape_info_[index].clear();
        for (auto s : input_shape) {
            input_shape_info_[index].push_back(s);
            input_tensor_size_info_[index] *= 1;
        }
    }

  private:
    void GetSessionInputOutput() {
        auto input_count  = session_->GetInputCount();
        auto output_count = session_->GetOutputCount();
        input_names_ptr_.reserve(input_count);
        output_names_ptr_.reserve(output_count);
        input_node_names_.reserve(input_count);
        output_node_names_.reserve(output_count);

        for (size_t ic = 0; ic < input_count; ic++) {
            auto input_name = session_->GetInputNameAllocated(ic, session_allocator_);
            auto typeinfo   = session_->GetInputTypeInfo(ic);
            auto tensorinfo = typeinfo.GetTensorTypeAndShapeInfo();

            input_node_names_.push_back(input_name.get());
            input_names_ptr_.push_back(std::move(input_name));
            auto input_shape  = tensorinfo.GetShape();
            bool is_dyn_shape = false;
            for (auto is : input_shape) {
                if (is <= 0) {
                    is_dyn_shape = true;
                    break;
                }
            }
            input_shape_info_.push_back(input_shape);

            if (!is_dyn_shape) {
                input_tensor_size_info_.push_back(tensorinfo.GetElementCount());
            } else {
                input_tensor_size_info_.push_back(0);
            }

            input_dtype_info_.push_back(tensorinfo.GetElementType());
        }

        for (size_t oc = 0; oc < output_count; oc++) {
            auto output_name     = session_->GetOutputNameAllocated(oc, session_allocator_);
            auto output_typeinfo = session_->GetOutputTypeInfo(oc);
            output_node_names_.push_back(output_name.get());
            output_names_ptr_.push_back(std::move(output_name));
        }
    }

  private:
    std::unique_ptr<Ort::Session>        session_;
    std::vector<const char *>            input_node_names_;
    std::vector<const char *>            output_node_names_;
    std::vector<Ort::AllocatedStringPtr> input_names_ptr_;
    std::vector<Ort::AllocatedStringPtr> output_names_ptr_;

    std::vector<std::vector<int64_t>>      input_shape_info_;
    std::vector<ONNXTensorElementDataType> input_dtype_info_;
    std::vector<size_t>                    input_tensor_size_info_;
    Ort::AllocatorWithDefaultOptions       session_allocator_;
};
