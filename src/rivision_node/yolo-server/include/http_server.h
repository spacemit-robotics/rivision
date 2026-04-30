/*
 * Copyright (C) 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
 * SPDX-License-Identifier: Apache-2.0
 */

#ifndef HTTP_SERVER_H
#define HTTP_SERVER_H

#include <functional>
#include <map>
#include <memory>
#include <string>

namespace http {

struct Request {
    std::string method;
    std::string path;
    std::map<std::string, std::string> headers;
    std::string body;
};

struct Response {
    int status_code = 200;
    std::map<std::string, std::string> headers;
    std::string body;

    void set_json(const std::string &json) {
        headers["Content-Type"] = "application/json";
        body = json;
    }
};

using RequestHandler = std::function<void(const Request &, Response &)>;

class Server {
public:
    Server();
    ~Server();

    void get(const std::string &path, RequestHandler handler);
    void post(const std::string &path, RequestHandler handler);

    bool listen(const std::string &host, int port);
    void stop();

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
};

}  // namespace http

#endif  // HTTP_SERVER_H
