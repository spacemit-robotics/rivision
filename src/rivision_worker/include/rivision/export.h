#pragma once

#ifdef _WIN32
    #ifdef RIVISION_EXPORT
        #define RIVISION_API __declspec(dllexport)
    #else
        #define RIVISION_API __declspec(dllimport)
    #endif
#else
    #define RIVISION_API __attribute__((visibility("default")))
#endif

#define RIVISION_VERSION_MAJOR 1
#define RIVISION_VERSION_MINOR 0
#define RIVISION_VERSION_PATCH 0
#define RIVISION_VERSION_STRING "1.0.0"
