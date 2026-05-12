// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#include "spine_llm_argparser.h"
#include <algorithm>
#include <cctype>
#include <fstream>
#include <iostream>
#include <sstream>

namespace onnxruntime {
namespace spacemit {
std::string SpineLLMArgParser::ReadFileToString(const std::string & path) {
    std::ifstream file(path);
    if (!file.is_open()) {
        std::cerr << "Error: Cannot open file: " << path << std::endl;
        return "";
    }
    std::ostringstream ss;
    ss << file.rdbuf();
    return ss.str();
}

std::string SpineLLMArgParser::Trim(const std::string & str) {
    size_t start = str.find_first_not_of(" \t\r\n");
    if (start == std::string::npos) {
        return "";
    }
    size_t end = str.find_last_not_of(" \t\r\n");
    return str.substr(start, end - start + 1);
}

std::string SpineLLMArgParser::ExtractStringValue(const std::string & content, const std::string & key) {
    std::string pattern = "\"" + key + "\"";
    size_t      pos     = content.find(pattern);
    if (pos == std::string::npos) {
        return "";
    }

    size_t colon = content.find(':', pos);
    if (colon == std::string::npos) {
        return "";
    }

    size_t start = content.find('"', colon);
    if (start == std::string::npos) {
        return "";
    }

    size_t end = content.find('"', start + 1);
    if (end == std::string::npos) {
        return "";
    }

    return content.substr(start + 1, end - start - 1);
}

int SpineLLMArgParser::ExtractIntValue(const std::string & content, const std::string & key, int default_val) {
    std::string pattern = "\"" + key + "\"";
    size_t      pos     = content.find(pattern);
    if (pos == std::string::npos) {
        return default_val;
    }

    size_t colon = content.find(':', pos);
    if (colon == std::string::npos) {
        return default_val;
    }

    size_t start = colon + 1;
    while (start < content.size() && std::isspace(static_cast<unsigned char>(content[start]))) {
        ++start;
    }

    size_t end = start;
    while (end < content.size() && (std::isdigit(static_cast<unsigned char>(content[end])) || content[end] == '-')) {
        ++end;
    }

    if (start == end) {
        return default_val;
    }

    std::string num_str = content.substr(start, end - start);
    try {
        return std::stoi(num_str);
    } catch (...) {
        return default_val;
    }
}

std::vector<std::string> SpineLLMArgParser::ExtractStringArray(const std::string & content, const std::string & key) {
    std::string pattern = "\"" + key + "\"";
    size_t      pos     = content.find(pattern);
    if (pos == std::string::npos) {
        return {};
    }

    size_t colon = content.find(':', pos);
    if (colon == std::string::npos) {
        return {};
    }

    size_t start_bracket = content.find('[', colon);
    if (start_bracket == std::string::npos) {
        return {};
    }

    size_t end_bracket = content.find(']', start_bracket);
    if (end_bracket == std::string::npos) {
        return {};
    }

    std::string              array_str = content.substr(start_bracket + 1, end_bracket - start_bracket - 1);
    std::vector<std::string> result;
    std::istringstream       iss(array_str);
    std::string              token;

    while (std::getline(iss, token, ',')) {
        token = Trim(token);
        if (token.size() >= 2 && token.front() == '"' && token.back() == '"') {
            result.push_back(token.substr(1, token.size() - 2));
        }
    }
    return result;
}

std::string SpineLLMArgParser::NormalizePath(const std::string & base_dir, const std::string & rel_path) {
    std::string clean_rel = rel_path;

    if (clean_rel.substr(0, 2) == "./") {
        clean_rel = clean_rel.substr(2);
    }

    if (!base_dir.empty() && base_dir.back() == '/') {
        return base_dir + clean_rel;
    } else {
        return base_dir + "/" + clean_rel;
    }
}

size_t SpineLLMArgParser::FindClosingBrace(const std::string & str, size_t startPos) {
    if (startPos >= str.size() || str[startPos] != '{') {
        return std::string::npos;
    }

    int depth = 0;
    for (size_t i = startPos; i < str.size(); ++i) {
        if (str[i] == '{') {
            depth++;
        } else if (str[i] == '}') {
            depth--;
            if (depth == 0) {
                return i;
            }
        }
        if (str[i] == '"' && (i == 0 || str[i - 1] != '\\')) {
            i++;
            while (i < str.size() && (str[i] != '"' || str[i - 1] == '\\')) {
                i++;
            }
        }
    }
    return std::string::npos;
}

int64_t SpineLLMArgParser::ExtractInt64Value(const std::string & json_block,
                                             const std::string & key,
                                             int64_t             default_value) {
    size_t key_pos = json_block.find("\"" + key + "\":");
    if (key_pos == std::string::npos) {
        return default_value;
    }

    size_t value_start = json_block.find_first_not_of(" \t\r\n", key_pos + key.length() + 3);  // skip key and ":"
    if (value_start == std::string::npos) {
        return default_value;
    }

    if (json_block[value_start] != '-' && !std::isdigit(json_block[value_start])) {
        return default_value;
    }

    char *    end;
    long long value = std::strtoll(json_block.c_str() + value_start, &end, 10);
    if (end == json_block.c_str() + value_start) {  // no conversion
        return default_value;
    }
    return static_cast<int64_t>(value);
}

bool SpineLLMArgParser::LoadConfigFromDir(const std::string & data_dir, SpineModelConfig & config) {
    std::string config_path = data_dir + "/config.json";
    std::string content     = ReadFileToString(config_path);
    if (content.empty()) {
        std::cerr << "Error: Failed to read config file: " << config_path << "\n";
        return false;
    }

    // --- Parse vision_model block ---
    size_t vision_start = content.find("\"vision_model\":");
    if (vision_start == std::string::npos) {
        std::cerr << "Error: 'vision_model' section not found.\n";
        return false;
    }
    size_t vision_block_start = content.find('{', vision_start);
    size_t vision_block_end   = FindClosingBrace(content, vision_block_start);
    if (vision_block_start == std::string::npos || vision_block_end == std::string::npos) {
        std::cerr << "Error: Invalid 'vision_model' block.\n";
        return false;
    }
    std::string vision_block    = content.substr(vision_block_start, vision_block_end - vision_block_start + 1);
    std::string raw_vision_path = ExtractStringValue(vision_block, "model_path");

    // --- Parse text_model block ---
    size_t text_start = content.find("\"text_model\":");
    if (text_start == std::string::npos) {
        std::cerr << "Error: 'text_model' section not found.\n";
        return false;
    }
    size_t text_block_start = content.find('{', text_start);
    size_t text_block_end   = FindClosingBrace(content, text_block_start);
    if (text_block_start == std::string::npos || text_block_end == std::string::npos) {
        std::cerr << "Error: Invalid 'text_model' block.\n";
        return false;
    }
    std::string text_block         = content.substr(text_block_start, text_block_end - text_block_start + 1);
    std::string raw_llm_path       = ExtractStringValue(text_block, "model_path");
    std::string raw_token_emb_path = ExtractStringValue(text_block, "token_embedding_path");

    if (raw_vision_path.empty() || raw_llm_path.empty() || raw_token_emb_path.empty()) {
        std::cerr << "Error: Missing required model paths.\n";
        return false;
    }

    // --- Set paths (normalize relative to data_dir) ---
    config.vision_model_path    = NormalizePath(data_dir, raw_vision_path);
    config.llm_model_path       = NormalizePath(data_dir, raw_llm_path);
    config.token_embedding_path = NormalizePath(data_dir, raw_token_emb_path);  // 新增

    // --- Extract scalar values from text_model block ---
    config.context_size       = ExtractIntValue(text_block, "context_size", 4096);
    config.logical_batch_size = ExtractIntValue(text_block, "logical_batch_size", 2048);
    config.predict_token      = ExtractIntValue(text_block, "predict_token", 128);
    config.vocab_size         = ExtractInt64Value(text_block, "vocab_size", 151936);  // 新增
    config.hidden_size        = ExtractInt64Value(text_block, "hidden_size", 896);    // 新增

    // --- Extract architectures from root ---
    config.architectures = ExtractStringArray(content, "architectures");

    return true;
}
}  // namespace spacemit
}  // namespace onnxruntime
