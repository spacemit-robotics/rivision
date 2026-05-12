// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#include "spine_llm_engine.h"
#include "spine_vision_engine.h"
#include <chrono>
#include <fstream>
#include <iostream>
#include <limits>
#include <random>

namespace onnxruntime {
const OrtApi * g_ort = NULL;
}  // namespace onnxruntime

int main(int argc, char * argv[]) {
    if (argc != 2) {
        std::cerr << "Usage: " << argv[0] << " <multimodal_data_dir>\n";
        return 1;
    }
    const std::string file_path = argv[1];

    onnxruntime::spacemit::SpineModelConfig config;
    onnxruntime::spacemit::SpineLLMArgParser::LoadConfigFromDir(file_path, config);

    onnxruntime::g_ort = OrtGetApiBase()->GetApi(ORT_API_VERSION);
    onnxruntime::spacemit::SpineVisionModelEngine vision_engine(config.vision_model_path);
    Ort::Session &                                vision_session = vision_engine.CreateVisionModelSession();

    onnxruntime::spacemit::SpineLLMEngine llm_engine(config.llm_model_path);
    bool                                  status = llm_engine.InitSpineLLMEngine(config);

    std::string input_binary_path;
    std::string user_text;

    const std::string system_prompt =
    "<|im_start|>system\n"
    "你是一个多模态助手。请结合用户提供的图片和问题进行回答。\n"
    "<|im_end|>\n";

    std::string user_prefix =
        "<|im_start|>user\n"
        "<image>\n";

    std::cout << "Please enter the path to the image binary file (e.g., input.bin): ";
    std::getline(std::cin, input_binary_path);
    std::cout << "Please enter your prompt in Chinese: ";
    std::getline(std::cin, user_text);

    std::string user_suffix =
        user_text + "\n"
        "<|im_end|>\n"
        "<|im_start|>assistant\n";
    std::string full_prompt = system_prompt + user_prefix + user_suffix;
    // // 拼接固定后缀
    Ort::Value &       vision_input_tensor = vision_engine.SetInputTensor(input_binary_path);
    std::vector<float> image_embedding     = vision_engine.RunSession(vision_input_tensor);
    llm_engine.Run(full_prompt, image_embedding, config);

    return 0;
}
