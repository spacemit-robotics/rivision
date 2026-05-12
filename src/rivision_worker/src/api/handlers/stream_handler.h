#pragma once

#include "rivision/types.h"
#include "rivision/config.h"
#include <string>
#include <functional>
#include <vector>

namespace rivision::api {

class StreamHandler {
public:
    using AddStreamCallback = std::function<bool(const StreamConfig&)>;
    using RemoveStreamCallback = std::function<bool(const StreamId&)>;
    using GetStreamsCallback = std::function<std::vector<StreamStatus>()>;
    using GetStreamCallback = std::function<StreamStatus(const StreamId&)>;
    
    StreamHandler();
    ~StreamHandler();
    
    void setAddStreamCallback(AddStreamCallback cb) { add_callback_ = std::move(cb); }
    void setRemoveStreamCallback(RemoveStreamCallback cb) { remove_callback_ = std::move(cb); }
    void setGetStreamsCallback(GetStreamsCallback cb) { get_all_callback_ = std::move(cb); }
    void setGetStreamCallback(GetStreamCallback cb) { get_callback_ = std::move(cb); }
    
    std::string handleListStreams();
    
    std::string handleGetStream(const std::string& id);
    
    std::string handleAddStream(const std::string& body);
    
    std::string handleRemoveStream(const std::string& id);
    
    std::string handleUpdateStream(const std::string& id, const std::string& body);
    
private:
    StreamConfig parseStreamConfig(const std::string& json);
    std::string streamStatusToJson(const StreamStatus& status);
    
    AddStreamCallback add_callback_;
    RemoveStreamCallback remove_callback_;
    GetStreamsCallback get_all_callback_;
    GetStreamCallback get_callback_;
};

} // namespace rivision::api
