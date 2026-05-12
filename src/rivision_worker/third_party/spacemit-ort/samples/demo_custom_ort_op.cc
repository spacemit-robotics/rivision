// SPDX-FileCopyrightText: Copyright (c) 2025 SpacemiT. All rights reserved.
// SPDX-License-Identifier: MIT

#include "demo_custom_ort_op.h"
#include "onnxruntime_c_api.h"
#include "onnxruntime_cxx_api.h"
#include "onnxruntime_lite_custom_op.h"
#include <cassert>
#include <cmath>
#include <mutex>
#include <system_error>
#include <vector>

static const char *c_OpDomain = "funasr.customop";

#define CUSTOM_ENFORCE(cond, msg)                                              \
  if (!(cond)) {                                                               \
    ORT_CXX_API_THROW(msg, OrtErrorCode::ORT_RUNTIME_EXCEPTION);               \
  }

namespace ASRCustomOp {

struct CifComputeContext {
  const float *hidden{nullptr};
  const float *alpha{nullptr};
  float *frame_fires{nullptr};
  float *fires{nullptr};
  int64_t *max_label_num{nullptr};
  float threshold{0};
  int64_t len_time{0};
  int64_t hidden_size{0};
};

void CifWoHidden_ComputeBatch(void *ctx, size_t i) {
  auto *cif_ctx = static_cast<CifComputeContext *>(ctx);
  auto threshold = cif_ctx->threshold;
  auto *y_cur = cif_ctx->fires + i * cif_ctx->len_time;
  auto *x_cur = cif_ctx->alpha + i * cif_ctx->len_time;
  auto *y_prev = y_cur;
  *y_cur = *x_cur;
  y_cur++;
  x_cur++;
  for (int64_t j = 1; j < cif_ctx->len_time; ++j) {
    auto prev = *y_prev;
    prev = prev >= threshold ? (prev - threshold * 1.0f) : prev;
    *y_cur = prev + *x_cur;

    y_cur++;
    x_cur++;
    y_prev++;
  }
}

void Cif_ComputeBatch(void *ctx, size_t i) {
  auto *cif_ctx = static_cast<CifComputeContext *>(ctx);
  auto threshold = cif_ctx->threshold;
  auto hidden_size = cif_ctx->hidden_size;
  auto len_time = cif_ctx->len_time;
  std::vector<bool> enable_seq(len_time, false);
  auto *fires_cur = cif_ctx->fires + i * len_time;
  auto *alpha_cur = cif_ctx->alpha + i * len_time;

  auto *hidden_cur = cif_ctx->hidden + i * len_time * hidden_size;
  auto *frame_fires_cur = cif_ctx->frame_fires + i * len_time * hidden_size;
  // 历史数据放输出的末尾
  auto *last_frames_cur = frame_fires_cur + hidden_size * (len_time - 1);

  auto *fires_prev = fires_cur;
  auto distribution_completion = 1.0f;
  *fires_cur = *alpha_cur;
  enable_seq[0] = *fires_cur >= threshold;

  auto cur = enable_seq[0] ? distribution_completion : *alpha_cur;
  auto remainds = *alpha_cur - cur;

  for (int64_t n = 0; n < hidden_size; ++n) {
    frame_fires_cur[n] = cur * hidden_cur[n];
  }

  if (len_time > 1) {
    if (enable_seq[0]) {
      for (int64_t n = 0; n < hidden_size; ++n) {
        last_frames_cur[n] = remainds * hidden_cur[n];
      }
    } else {
      memcpy(last_frames_cur, frame_fires_cur, hidden_size * sizeof(float));
    }
  }

  fires_cur++;
  alpha_cur++;
  hidden_cur += hidden_size;
  frame_fires_cur += hidden_size;

  for (int64_t j = 1; j < len_time; ++j) {
    auto prev = *fires_prev;
    prev = prev >= threshold ? (prev - 1.0f) : prev;

    distribution_completion = 1.0f - prev;

    *fires_cur = prev + *alpha_cur;
    enable_seq[j] = *fires_cur >= threshold;

    cur = enable_seq[j] ? distribution_completion : *alpha_cur;
    remainds = *alpha_cur - cur;
    for (int64_t n = 0; n < hidden_size; ++n) {
      frame_fires_cur[n] = cur * hidden_cur[n] + last_frames_cur[n];
    }

    if (j < len_time - 1) {
      // 最后一次不更新
      if (enable_seq[j]) {
        for (int64_t n = 0; n < hidden_size; ++n) {
          last_frames_cur[n] = remainds * hidden_cur[n];
        }
      } else {
        memcpy(last_frames_cur, frame_fires_cur, hidden_size * sizeof(float));
      }
    }

    fires_cur++;
    alpha_cur++;
    fires_prev++;
    hidden_cur += hidden_size;
    frame_fires_cur += hidden_size;
  }

  frame_fires_cur = cif_ctx->frame_fires + i * len_time * hidden_size;
  auto *dst_frame_fires_cur = frame_fires_cur;
  int64_t max_len = 0;
  for (auto cp_enable : enable_seq) {
    if (cp_enable) {
      memcpy(dst_frame_fires_cur, frame_fires_cur, hidden_size * sizeof(float));
      dst_frame_fires_cur += hidden_size;
      max_len++;
    }

    frame_fires_cur += hidden_size;
  }
  memset(dst_frame_fires_cur, 0,
         hidden_size * sizeof(float) * (len_time - max_len));
  cif_ctx->max_label_num[i] = max_len;
}

struct CifWoHidden_Kernel {
  CifWoHidden_Kernel(const OrtApi *ort_api, const OrtKernelInfo *info) {
    auto sts =
        ort_api->KernelInfoGetAttribute_float(info, "threshold", &threshold_);
  }

  Ort::Status Compute(OrtKernelContext *context,
                      const Ort::Custom::Tensor<float> &X,
                      Ort::Custom::Tensor<float> &Y) {
    Ort::KernelContext ctx(context);
    auto input_shape = X.Shape();
    auto x_raw = X.Data();
    CUSTOM_ENFORCE(input_shape.size() == 2, "input shape should be rank 2");

    auto batch_size = input_shape[0];
    auto len_time = input_shape[1];

    auto y_raw = Y.Allocate(input_shape);

    if (X.NumberOfElement() == 0) {
      return Ort::Status{nullptr};
    }

    CifComputeContext cif_ctx;
    cif_ctx.alpha = x_raw;
    cif_ctx.fires = y_raw;
    cif_ctx.threshold = threshold_;
    cif_ctx.len_time = len_time;
    ctx.ParallelFor(CifWoHidden_ComputeBatch, batch_size, 1, &cif_ctx);

    return Ort::Status{nullptr};
  }

  float threshold_ = 0.0f;
};

struct Cif_Kernel {
  Cif_Kernel(const OrtApi *ort_api, const OrtKernelInfo *info) {
    auto sts =
        ort_api->KernelInfoGetAttribute_float(info, "threshold", &threshold_);
  }

  Ort::Status Compute(OrtKernelContext *context,
                      const Ort::Custom::Tensor<float> &hidden,
                      const Ort::Custom::Tensor<float> &alphas,
                      Ort::Custom::Tensor<float> &frame_fires,
                      Ort::Custom::Tensor<float> &fires,
                      Ort::Custom::Tensor<int64_t> &max_label) {
    Ort::KernelContext ctx(context);

    auto hidden_input_shape = hidden.Shape();
    auto hidden_raw = hidden.Data();
    auto alphas_input_shape = alphas.Shape();
    auto alphas_raw = alphas.Data();
    CUSTOM_ENFORCE(hidden_input_shape.size() == 3,
                   "hidden input shape should be rank 3");
    CUSTOM_ENFORCE(alphas_input_shape.size() == 2,
                   "alphas input shape should be rank 2");

    auto batch_size = hidden_input_shape[0];
    auto len_time = hidden_input_shape[1];
    auto hidden_size = hidden_input_shape[2];

    auto frame_fires_raw = frame_fires.Allocate(hidden_input_shape);
    auto fires_raw = fires.Allocate(alphas_input_shape);
    auto max_label_raw = max_label.Allocate({1});

    if (hidden.NumberOfElement() == 0) {
      return Ort::Status{nullptr};
    }

    std::vector<int64_t> batch_max_label_len(batch_size, 0);
    CifComputeContext cif_ctx;
    cif_ctx.hidden = hidden_raw;
    cif_ctx.alpha = alphas_raw;
    cif_ctx.frame_fires = frame_fires_raw;
    cif_ctx.fires = fires_raw;
    cif_ctx.max_label_num = batch_max_label_len.data();
    cif_ctx.threshold = threshold_;
    cif_ctx.len_time = len_time;
    cif_ctx.hidden_size = hidden_size;

    ctx.ParallelFor(
        reinterpret_cast<void (*)(void *, size_t)>(&Cif_ComputeBatch),
        batch_size, 1, &cif_ctx);

    *max_label_raw = *std::max_element(batch_max_label_len.begin(),
                                       batch_max_label_len.end());
    return Ort::Status{nullptr};
  }

  float threshold_ = 0.0f;
};
} // namespace ASRCustomOp

static void AddOrtCustomOpDomainToContainer(Ort::CustomOpDomain &&domain) {
  static std::vector<Ort::CustomOpDomain> ort_custom_op_domain_container;
  static std::mutex ort_custom_op_domain_mutex;
  std::lock_guard<std::mutex> lock(ort_custom_op_domain_mutex);
  ort_custom_op_domain_container.push_back(std::move(domain));
}

OrtStatus *ORT_API_CALL RegisterCustomOps(OrtSessionOptions *options,
                                          const OrtApiBase *api) {
  auto api_ = api->GetApi(ORT_API_VERSION);
  OrtStatus *result = nullptr;

  static const std::unique_ptr<Ort::Custom::OrtLiteCustomOp> CifWoHiddenOp{
      Ort::Custom::CreateLiteCustomOp<ASRCustomOp::CifWoHidden_Kernel>(
          "CifWoHidden", "CPUExecutionProvider", 1)};

  static const std::unique_ptr<Ort::Custom::OrtLiteCustomOp> CifOp{
      Ort::Custom::CreateLiteCustomOp<ASRCustomOp::Cif_Kernel>(
          "Cif", "CPUExecutionProvider", 1)};

  Ort::CustomOpDomain domain{c_OpDomain};
  domain.Add(CifWoHiddenOp.get());
  domain.Add(CifOp.get());

  Ort::UnownedSessionOptions session_options(options);
  session_options.Add(domain);
  AddOrtCustomOpDomainToContainer(std::move(domain));

  return result;
}
