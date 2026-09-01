# Code generated from vault's typed ops by opgen. DO NOT EDIT.
cmake_minimum_required(VERSION 3.16)
project(vault LANGUAGES CXX)

find_package(nlohmann_json 3 REQUIRED)

add_library(vault INTERFACE)
target_include_directories(vault INTERFACE ${CMAKE_CURRENT_SOURCE_DIR}/include)
target_compile_features(vault INTERFACE cxx_std_20)
target_link_libraries(vault INTERFACE nlohmann_json::nlohmann_json)
