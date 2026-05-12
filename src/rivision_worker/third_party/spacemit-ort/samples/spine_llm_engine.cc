// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#include "spine_llm_engine.h"
#include <algorithm>
#include <cassert>
#include <fstream>
#include <iostream>
#include <string>
#include <unordered_map>
#include <vector>

namespace onnxruntime {
namespace spacemit {

namespace {
static const std::unordered_map<std::string, SpineArchitecture> spine_architecture_map = {
    { "LlavaQwen2ForCausalLM", SpineArchitecture::LLaVA_Qwen2_ForCausalLM },
    // TODO ：
    // {"QwenVL", SpineArchitecture::QwenVL},
};

SpineArchitecture GetArchitectureFromString(const std::string & arch_name) {
    auto it = spine_architecture_map.find(arch_name);
    if (it != spine_architecture_map.end()) {
        return it->second;
    }
    return SpineArchitecture::UNKNOWN;
}
}  // namespace

bool SpineLLMEngine::InitSpineLLMEngine(SpineModelConfig config) {
    llama_params_       = llama_model_default_params();
    llm_model_ptr_      = llama_model_load_from_file(llm_model_path_.c_str(), llama_params_);
    llama_vocab_        = llama_model_get_vocab(llm_model_ptr_);
    ctx_params_         = llama_context_default_params();
    ctx_params_.n_ctx   = 2048;
    ctx_params_.n_batch = 2048;
    llm_context_ptr_    = llama_init_from_model(llm_model_ptr_, ctx_params_);

    // initialize the sampler
    llm_smpl_ = llama_sampler_chain_init(llama_sampler_chain_default_params());
    llama_sampler_chain_add(llm_smpl_, llama_sampler_init_min_p(0.05f, 1));
    llama_sampler_chain_add(llm_smpl_, llama_sampler_init_temp(0.75f));  // Balanced for detailed but stable output

    // Add repetition penalty to prevent repetitive output
    llama_sampler_chain_add(llm_smpl_, llama_sampler_init_penalties(
        64,      // penalty_last_n: penalize last 64 tokens
        1.15f,   // penalty_repeat: slightly stronger penalty for more varied descriptions
        0.0f,    // penalty_freq: disabled
        0.0f     // penalty_present: disabled
    ));

    llama_sampler_chain_add(llm_smpl_, llama_sampler_init_dist(LLAMA_DEFAULT_SEED));

    if (!llm_model_ptr_ || !llm_context_ptr_ || !llm_smpl_) {
        throw std::runtime_error("Failed to load model\n");
        return 1;
    }

    return 0;
}

void SpineLLMEngine::SetTokenEmbeddings(SpineModelConfig config) {
    const std::string embedding_file_path = config.token_embedding_path;
    vocab_size_                           = config.vocab_size;
    hidden_size_                          = config.hidden_size;

    const size_t total_elements = vocab_size_ * hidden_size_;
    token_embeddings_.resize(total_elements);

    std::ifstream file(embedding_file_path, std::ios::binary);
    if (!file.is_open()) {
        throw std::runtime_error("Failed to open embedding file: " + embedding_file_path);
    }

    file.read(reinterpret_cast<char *>(token_embeddings_.data()), total_elements * sizeof(float));

    if (!file) {
        throw std::runtime_error("Error reading embedding file. Expected " +
                                 std::to_string(total_elements * sizeof(float)) +
                                 " bytes, but file is incomplete or corrupted.");
    }

    file.close();
}

std::vector<float> & SpineLLMEngine::EncodeUserText(const std::string & text) {
    // === 0. 分割文本：找到 <image> 的位置，分成两部分 ===
    std::string before_image;
    std::string after_image;

    size_t image_pos = text.find("<image>");
    if (image_pos != std::string::npos) {
        before_image = text.substr(0, image_pos);
        size_t after_pos = image_pos + 7;  // "<image>" 的长度
        // 如果 <image> 后面紧跟换行符，跳过它
        if (after_pos < text.length() && text[after_pos] == '\n') {
            after_pos++;
        }
        after_image = text.substr(after_pos);

        fprintf(stderr, "Split text at <image> position:\n");
        fprintf(stderr, "  Before: '%s'\n", before_image.c_str());
        fprintf(stderr, "  After: '%s'\n", after_image.c_str());
    } else {
        // 如果没有 <image>，整个文本作为 before
        before_image = text;
        fprintf(stderr, "Warning: No <image> token found in text\n");
    }

    // === 1. Tokenize 两部分 ===
    const llama_vocab * vocab = llama_model_get_vocab(llm_model_ptr_);

    // Tokenize before_image (包含 BOS)
    std::vector<llama_token> before_tokens(before_image.size() + 10);
    int n_before = llama_tokenize(vocab, before_image.c_str(), static_cast<int>(before_image.length()),
                                  before_tokens.data(), static_cast<int>(before_tokens.size()),
                                  true,  // add BOS
                                  true   // parse special
    );
    if (n_before < 0) {
        before_tokens.resize(-n_before);
        n_before = llama_tokenize(vocab, before_image.c_str(), static_cast<int>(before_image.length()),
                                  before_tokens.data(), static_cast<int>(before_tokens.size()), true, true);
    }
    before_tokens.resize(n_before);

    // Tokenize after_image (不包含 BOS)
    std::vector<llama_token> after_tokens(after_image.size() + 10);
    int n_after = 0;
    if (!after_image.empty()) {
        n_after = llama_tokenize(vocab, after_image.c_str(), static_cast<int>(after_image.length()),
                                after_tokens.data(), static_cast<int>(after_tokens.size()),
                                false,  // no BOS (已经在 before 部分添加了)
                                true    // parse special
        );
        if (n_after < 0) {
            after_tokens.resize(-n_after);
            n_after = llama_tokenize(vocab, after_image.c_str(), static_cast<int>(after_image.length()),
                                    after_tokens.data(), static_cast<int>(after_tokens.size()), false, true);
        }
        after_tokens.resize(n_after);
    }

    int total_tokens = n_before + n_after;

    // 保存分割信息供 MultimodalEmbeddingFusion 使用
    n_tokens_before_image_ = n_before;
    n_tokens_after_image_ = n_after;

    // === Debug: 打印 token 信息 ===
    fprintf(stderr, "Tokenization result: %d tokens (%d before + %d after)\n",
            total_tokens, n_before, n_after);
    fprintf(stderr, "Before tokens:\n");
    for (int i = 0; i < std::min(n_before, 10); ++i) {
        char buf[256];
        int n = llama_token_to_piece(vocab, before_tokens[i], buf, sizeof(buf), 0, true);
        std::string piece(buf, std::max(0, n));
        fprintf(stderr, "  Token[%d] = %d ('%s')\n", i, before_tokens[i], piece.c_str());
    }
    if (n_before > 10) fprintf(stderr, "  ... (%d more)\n", n_before - 10);

    fprintf(stderr, "After tokens:\n");
    for (int i = 0; i < std::min(n_after, 10); ++i) {
        char buf[256];
        int n = llama_token_to_piece(vocab, after_tokens[i], buf, sizeof(buf), 0, true);
        std::string piece(buf, std::max(0, n));
        fprintf(stderr, "  Token[%d] = %d ('%s')\n", i, after_tokens[i], piece.c_str());
    }
    if (n_after > 10) fprintf(stderr, "  ... (%d more)\n", n_after - 10);

    // === 2. Encode：合并 before 和 after 的 embeddings ===
    text_embeddings_.resize(total_tokens * hidden_size_);

    // Copy before_tokens embeddings
    for (int i = 0; i < n_before; ++i) {
        llama_token   token_id = before_tokens[i];
        const float * src      = &token_embeddings_[token_id * hidden_size_];
        float *       dst      = &text_embeddings_[i * hidden_size_];
        std::copy_n(src, hidden_size_, dst);
    }

    // Copy after_tokens embeddings
    for (int i = 0; i < n_after; ++i) {
        llama_token   token_id = after_tokens[i];
        const float * src      = &token_embeddings_[token_id * hidden_size_];
        float *       dst      = &text_embeddings_[(n_before + i) * hidden_size_];
        std::copy_n(src, hidden_size_, dst);
    }

    return text_embeddings_;
}

std::vector<float> & SpineLLMEngine::MultimodalEmbeddingFusion(const std::vector<float> & image_emb,
                                                               const std::vector<float> & text_emb,
                                                               std::string &              architecture) {
    switch (spine_architecture_map.at(architecture)) {
        case SpineArchitecture::LLaVA_Qwen2_ForCausalLM:
            {
                size_t n_image = image_emb.size() / hidden_size_;
                size_t n_text  = text_emb.size() / hidden_size_;

                // LLaVA 架构的正确序列：[Before Tokens] [Image Features] [After Tokens]
                // 其中 Before Tokens 包含：[BOS] [<|im_start|>system...] [<|im_start|>user]
                // Image Features 替换 <image> token 的位置
                // After Tokens 包含：[question tokens] [<|im_end|>] [<|im_start|>assistant]

                int n_before = n_tokens_before_image_;
                int n_after = n_tokens_after_image_;

                fprintf(stderr, "Multimodal fusion breakdown:\n");
                fprintf(stderr, "  Text embeddings: %zu tokens (%d before + %d after)\n",
                       n_text, n_before, n_after);
                fprintf(stderr, "  Image embeddings: %zu tokens\n", n_image);

                // 验证 text_emb 大小是否匹配
                if (n_text != static_cast<size_t>(n_before + n_after)) {
                    throw std::runtime_error("Text embedding size mismatch: expected " +
                                           std::to_string(n_before + n_after) +
                                           " but got " + std::to_string(n_text));
                }

                // 总大小：n_before + n_image + n_after
                size_t total_tokens = n_before + n_image + n_after;
                multimodal_embeddings_.resize(total_tokens * hidden_size_);

                size_t offset = 0;

                // 1. Before image embeddings
                std::copy_n(text_emb.begin(), n_before * hidden_size_,
                           multimodal_embeddings_.begin() + offset);
                offset += n_before * hidden_size_;

                // 2. Image embeddings
                std::copy(image_emb.begin(), image_emb.end(),
                         multimodal_embeddings_.begin() + offset);
                offset += image_emb.size();

                // 3. After image embeddings
                if (n_after > 0) {
                    std::copy(text_emb.begin() + n_before * hidden_size_, text_emb.end(),
                             multimodal_embeddings_.begin() + offset);
                }

                fprintf(stderr, "Multimodal fusion result: [Before(%d)] + [Image(%zu)] + [After(%d)] = %zu tokens\n",
                       n_before, n_image, n_after, total_tokens);

                break;
            }
        default:
            throw std::runtime_error("We are not supporting this architecture now!");
            break;
    }

    return multimodal_embeddings_;
}

bool SpineLLMEngine::LLMPrefill(llama_context *      ctx,
                                std::vector<float> & embeddings,
                                int                  hidden_size,
                                llama_pos &          n_past) {
    size_t n_tokens = embeddings.size() / hidden_size;
    if (embeddings.size() != n_tokens * hidden_size) {
        throw std::runtime_error("Embedding size mismatch!\n");
        return false;
    }

    llama_batch batch = llama_batch_init(static_cast<int>(n_tokens), 0, 1);
    batch.n_tokens    = static_cast<int>(n_tokens);
    batch.embd        = embeddings.data();
    batch.token       = nullptr;

    for (size_t i = 0; i < n_tokens; ++i) {
        batch.pos[i]       = n_past + static_cast<llama_pos>(i);
        batch.n_seq_id[i]  = 1;
        batch.seq_id[i][0] = 0;
        batch.logits[i]    = false;
    }

    if (n_tokens > 0) {
        batch.logits[n_tokens - 1] = true;
    }

    int ret = llama_decode(ctx, batch);

    if (ret != 0) {
        throw std::runtime_error("llama_decode failed\n");
        return false;
    }

    n_past += n_tokens;

    return true;
}

std::string SpineLLMEngine::LLMDecode(llama_context * ctx, llama_model * model, llama_pos n_past) {
    llama_batch batch = llama_batch_init(1, 0, 1);
    llama_token token_id;

    std::string generated_text;  // Accumulate output here
    generated_text.reserve(4096);  // Pre-allocate to reduce reallocations

    const int max_tokens = 1024;  // Extended for more detailed descriptions
    int generated_tokens = 0;

    while (true) {
        // Check maximum generation length
        if (generated_tokens >= max_tokens) {
            fprintf(stderr, "\nMax generation length (%d tokens) reached\n", max_tokens);
            break;
        }

        // check if we have enough space in the context to evaluate this batch
        int n_ctx      = llama_n_ctx(ctx);
        int n_ctx_used = llama_memory_seq_pos_max(llama_get_memory(ctx), 0) + 1;
        if (n_ctx_used + batch.n_tokens > n_ctx) {
            fprintf(stderr, "context size exceeded\n");
            break;
        }

        token_id = llama_sampler_sample(llm_smpl_, ctx, -1);
        if (llama_vocab_is_eog(llama_vocab_, token_id)) {
            break;
        }
        // output
        char buf[256];
        int  n = llama_token_to_piece(llama_vocab_, token_id, buf, sizeof(buf), 0, true);
        if (n < 0) {
            GGML_ABORT("failed to convert token to piece\n");
        }
        std::string piece(buf, n);

        // Accumulate in string buffer instead of printing
        generated_text += piece;

        // Optional: still print to stderr for debugging (not stdout)
        fprintf(stderr, "%s", piece.c_str());
        fflush(stderr);

        batch = llama_batch_get_one(&token_id, 1);

        if (llama_decode(ctx, batch) != 0) {
            throw std::runtime_error("Decode failed during generation!\n");
            break;
        }

        generated_tokens++;
    }

    fprintf(stderr, "\n");
    return generated_text;
}

std::string SpineLLMEngine::Run(std::string & user_text, std::vector<float> & image_embedding, SpineModelConfig config) {
    // Clear KV cache before each inference to ensure clean state
    llama_memory_t mem = llama_get_memory(llm_context_ptr_);
    llama_memory_seq_rm(mem, -1, 0, -1);  // Remove all sequences

    SetTokenEmbeddings(config);
    auto & text_embeddings = EncodeUserText(user_text);
    if (config.architectures.size() == 0) {
        throw std::invalid_argument("architectures is empty");
    }
    std::vector<float> & multimodal_emb =
        MultimodalEmbeddingFusion(image_embedding, text_embeddings, config.architectures[0]);
    llama_pos n_past = 0;
    LLMPrefill(llm_context_ptr_, multimodal_emb, hidden_size_, n_past);
    std::string generated_text = LLMDecode(llm_context_ptr_, llm_model_ptr_, n_past);
    return generated_text;
}
}  // namespace spacemit
}  // namespace onnxruntime
