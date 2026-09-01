// Code generated from vault's typed ops by opgen. DO NOT EDIT.
//
// vault's API.
//
// A field absent from an answer keeps its default. The service is written
// in Go, where a struct field is always present and an absent one arrives as
// its zero value, so refusing an answer for a missing key would refuse
// answers the service considers complete.
#pragma once

#include <cstdint>
#include <map>
#include <memory>
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>

#include <nlohmann/json.hpp>

namespace vault {

/// One answer, as bytes.
struct Reply {
    int status = 0;
    std::string body;
};

/// How the bytes travel. The SDK knows the contract; the program it lands in
/// already has an HTTP client, and this is where it plugs in.
///
/// `target` is an absolute path with its query string, so the base
/// address belongs to the implementation and not to the call.
struct Transport {
    virtual ~Transport() = default;
    virtual Reply send(const std::string& method, const std::string& target,
                       const std::string* body) = 0;
};

/// The service answered, and refused.
class Refused : public std::runtime_error {
 public:
    Refused(int status, std::string body)
        : std::runtime_error("status " + std::to_string(status) + ": " + body),
          status_(status), body_(std::move(body)) {}

    int status() const noexcept { return status_; }
    const std::string& body() const noexcept { return body_; }

 private:
    int status_;
    std::string body_;
};

/// Percent-encodes one path segment or query value. Everything outside the
/// unreserved set is escaped, which is correct in both places.
inline std::string encode(const std::string& s) {
    static const char* hex = "0123456789ABCDEF";
    std::string out;
    out.reserve(s.size());
    for (unsigned char c : s) {
        if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
            (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~') {
            out.push_back(static_cast<char>(c));
        } else {
            out.push_back('%');
            out.push_back(hex[c >> 4]);
            out.push_back(hex[c & 0x0F]);
        }
    }
    return out;
}

/// Spells one number the way the URL carries it.
template <typename T>
inline std::string number(const T& v) {
    std::ostringstream out;
    out << v;
    return out.str();
}

/// Spells a repeated value the way the service reads one: comma separated.
template <typename T>
inline std::string join(const std::vector<T>& vs) {
    std::string out;
    for (std::size_t i = 0; i < vs.size(); ++i) {
        if (i > 0) out.push_back(',');
        out += number(vs[i]);
    }
    return out;
}

/// A string joins as itself, not through a stream.
template <>
inline std::string join<std::string>(const std::vector<std::string>& vs) {
    std::string out;
    for (std::size_t i = 0; i < vs.size(); ++i) {
        if (i > 0) out.push_back(',');
        out += vs[i];
    }
    return out;
}

// The types this service moves.
struct Party;
struct Secret;
struct Held;
struct Ready;
struct Seal;
struct Sealed;
struct VaultList;
struct VaultRead;

struct Party {
    std::string kind{};
    std::string org{};
};

struct Secret {
    std::string at{};
    std::string name{};
    Party owner{};
};

struct Held {
    bool more{};
    std::vector<Secret> secrets{};
};

struct Ready {
    std::string name{};
    bool ready{};
};

struct Seal {
    std::string at{};
    std::string blob{};
    std::int64_t bytes{};
    std::vector<std::int32_t> counts{};
    nlohmann::json free{};
    std::map<std::string, std::string> labels{};
    std::string name{};
    Party nested{};
    Party owner{};
    bool rotate{};
    std::vector<std::string> tags{};
    double weight{};
};

struct Sealed {
    std::string at{};
    std::string ref{};
};

/// List the secrets this vault holds
struct VaultList {
    std::string org{};
    std::int64_t limit{};
};

/// Read one secret's record by name
struct VaultRead {
    std::string name{};
    bool reveal{};
};

inline void to_json(nlohmann::json& j, const Party& v);
inline void from_json(const nlohmann::json& j, Party& v);
inline void to_json(nlohmann::json& j, const Secret& v);
inline void from_json(const nlohmann::json& j, Secret& v);
inline void to_json(nlohmann::json& j, const Held& v);
inline void from_json(const nlohmann::json& j, Held& v);
inline void to_json(nlohmann::json& j, const Ready& v);
inline void from_json(const nlohmann::json& j, Ready& v);
inline void to_json(nlohmann::json& j, const Seal& v);
inline void from_json(const nlohmann::json& j, Seal& v);
inline void to_json(nlohmann::json& j, const Sealed& v);
inline void from_json(const nlohmann::json& j, Sealed& v);
inline void to_json(nlohmann::json& j, const VaultList& v);
inline void from_json(const nlohmann::json& j, VaultList& v);
inline void to_json(nlohmann::json& j, const VaultRead& v);
inline void from_json(const nlohmann::json& j, VaultRead& v);

inline void to_json(nlohmann::json& j, const Party& v) {
    j = nlohmann::json::object();
    j["kind"] = v.kind;
    j["org"] = v.org;
}

inline void from_json(const nlohmann::json& j, Party& v) {
    if (auto it = j.find("kind"); it != j.end() && !it->is_null()) {
        it->get_to(v.kind);
    }
    if (auto it = j.find("org"); it != j.end() && !it->is_null()) {
        it->get_to(v.org);
    }
}

inline void to_json(nlohmann::json& j, const Secret& v) {
    j = nlohmann::json::object();
    j["at"] = v.at;
    j["name"] = v.name;
    j["owner"] = v.owner;
}

inline void from_json(const nlohmann::json& j, Secret& v) {
    if (auto it = j.find("at"); it != j.end() && !it->is_null()) {
        it->get_to(v.at);
    }
    if (auto it = j.find("name"); it != j.end() && !it->is_null()) {
        it->get_to(v.name);
    }
    if (auto it = j.find("owner"); it != j.end() && !it->is_null()) {
        it->get_to(v.owner);
    }
}

inline void to_json(nlohmann::json& j, const Held& v) {
    j = nlohmann::json::object();
    j["more"] = v.more;
    j["secrets"] = v.secrets;
}

inline void from_json(const nlohmann::json& j, Held& v) {
    if (auto it = j.find("more"); it != j.end() && !it->is_null()) {
        it->get_to(v.more);
    }
    if (auto it = j.find("secrets"); it != j.end() && !it->is_null()) {
        it->get_to(v.secrets);
    }
}

inline void to_json(nlohmann::json& j, const Ready& v) {
    j = nlohmann::json::object();
    j["name"] = v.name;
    j["ready"] = v.ready;
}

inline void from_json(const nlohmann::json& j, Ready& v) {
    if (auto it = j.find("name"); it != j.end() && !it->is_null()) {
        it->get_to(v.name);
    }
    if (auto it = j.find("ready"); it != j.end() && !it->is_null()) {
        it->get_to(v.ready);
    }
}

inline void to_json(nlohmann::json& j, const Seal& v) {
    j = nlohmann::json::object();
    j["at"] = v.at;
    j["blob"] = v.blob;
    j["bytes"] = v.bytes;
    j["counts"] = v.counts;
    j["free"] = v.free;
    j["labels"] = v.labels;
    j["name"] = v.name;
    j["nested"] = v.nested;
    j["owner"] = v.owner;
    j["rotate"] = v.rotate;
    j["tags"] = v.tags;
    j["weight"] = v.weight;
}

inline void from_json(const nlohmann::json& j, Seal& v) {
    if (auto it = j.find("at"); it != j.end() && !it->is_null()) {
        it->get_to(v.at);
    }
    if (auto it = j.find("blob"); it != j.end() && !it->is_null()) {
        it->get_to(v.blob);
    }
    if (auto it = j.find("bytes"); it != j.end() && !it->is_null()) {
        it->get_to(v.bytes);
    }
    if (auto it = j.find("counts"); it != j.end() && !it->is_null()) {
        it->get_to(v.counts);
    }
    if (auto it = j.find("free"); it != j.end() && !it->is_null()) {
        it->get_to(v.free);
    }
    if (auto it = j.find("labels"); it != j.end() && !it->is_null()) {
        it->get_to(v.labels);
    }
    if (auto it = j.find("name"); it != j.end() && !it->is_null()) {
        it->get_to(v.name);
    }
    if (auto it = j.find("nested"); it != j.end() && !it->is_null()) {
        it->get_to(v.nested);
    }
    if (auto it = j.find("owner"); it != j.end() && !it->is_null()) {
        it->get_to(v.owner);
    }
    if (auto it = j.find("rotate"); it != j.end() && !it->is_null()) {
        it->get_to(v.rotate);
    }
    if (auto it = j.find("tags"); it != j.end() && !it->is_null()) {
        it->get_to(v.tags);
    }
    if (auto it = j.find("weight"); it != j.end() && !it->is_null()) {
        it->get_to(v.weight);
    }
}

inline void to_json(nlohmann::json& j, const Sealed& v) {
    j = nlohmann::json::object();
    j["at"] = v.at;
    j["ref"] = v.ref;
}

inline void from_json(const nlohmann::json& j, Sealed& v) {
    if (auto it = j.find("at"); it != j.end() && !it->is_null()) {
        it->get_to(v.at);
    }
    if (auto it = j.find("ref"); it != j.end() && !it->is_null()) {
        it->get_to(v.ref);
    }
}

inline void to_json(nlohmann::json& j, const VaultList& v) {
    j = nlohmann::json::object();
    j["org"] = v.org;
    j["limit"] = v.limit;
}

inline void from_json(const nlohmann::json& j, VaultList& v) {
    if (auto it = j.find("org"); it != j.end() && !it->is_null()) {
        it->get_to(v.org);
    }
    if (auto it = j.find("limit"); it != j.end() && !it->is_null()) {
        it->get_to(v.limit);
    }
}

inline void to_json(nlohmann::json& j, const VaultRead& v) {
    j = nlohmann::json::object();
    j["name"] = v.name;
    j["reveal"] = v.reveal;
}

inline void from_json(const nlohmann::json& j, VaultRead& v) {
    if (auto it = j.find("name"); it != j.end() && !it->is_null()) {
        it->get_to(v.name);
    }
    if (auto it = j.find("reveal"); it != j.end() && !it->is_null()) {
        it->get_to(v.reveal);
    }
}

/// vault's operations.
class Client {
 public:
    explicit Client(Transport& transport) : transport_(transport) {}

    /// Report whether this replica is serving
    ///
    /// GET /v1/health
    Ready vault_health() {
        std::string target = "/v1/health";
        const Reply reply = transport_.send("GET", target, nullptr);
        if (reply.status < 200 || reply.status >= 300) {
            throw Refused(reply.status, reply.body);
        }
        return nlohmann::json::parse(reply.body).get<Ready>();
    }

    /// List the secrets this vault holds
    ///
    /// GET /v1/secrets
    Held vault_list(const VaultList& input) {
        std::string target = "/v1/secrets";
        target += "?org=";
        target += encode(input.org);
        target += "&limit=";
        target += encode(number(input.limit));
        const Reply reply = transport_.send("GET", target, nullptr);
        if (reply.status < 200 || reply.status >= 300) {
            throw Refused(reply.status, reply.body);
        }
        return nlohmann::json::parse(reply.body).get<Held>();
    }

    /// Read one secret's record by name
    ///
    /// GET /v1/secrets/{name}
    Secret vault_read(const VaultRead& input) {
        std::string target;
        target += "/v1/secrets/";
        target += encode(input.name);
        target += "?reveal=";
        target += encode(std::string(input.reveal ? "true" : "false"));
        const Reply reply = transport_.send("GET", target, nullptr);
        if (reply.status < 200 || reply.status >= 300) {
            throw Refused(reply.status, reply.body);
        }
        return nlohmann::json::parse(reply.body).get<Secret>();
    }

    /// Seal a secret. The value is never readable again.
    ///
    /// POST /v1/seal
    Sealed vault_seal(const Seal& input) {
        std::string target = "/v1/seal";
        nlohmann::json body = input;
        const std::string encoded = body.dump();
        const Reply reply = transport_.send("POST", target, &encoded);
        if (reply.status < 200 || reply.status >= 300) {
            throw Refused(reply.status, reply.body);
        }
        return nlohmann::json::parse(reply.body).get<Sealed>();
    }

 private:
    Transport& transport_;
};

}  // namespace vault
