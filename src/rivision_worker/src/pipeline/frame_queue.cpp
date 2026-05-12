/**
 * @file frame_queue.cpp
 * @brief Frame queue implementation (lock-free ring buffer)
 */

#include "frame_queue.h"

namespace rivision {
namespace pipeline {

// FrameQueue is a template class, so most implementation is in the header.
// This file provides explicit template instantiations for common types.

// Explicit instantiation for rivision::Frame
template class FrameQueue<rivision::Frame>;

// Explicit instantiation for hal::StreamPacket
template class FrameQueue<hal::StreamPacket>;

// Explicit instantiation for InferenceResult (if needed)
// template class FrameQueue<InferenceResult>;

} // namespace pipeline
} // namespace rivision
