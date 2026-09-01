package opgen_test

// The programs the compile tests run.
//
// They are written here rather than generated, deliberately. A generator whose
// output is only ever checked by more generated code checks that it agrees with
// itself. These are hand-written against the fixture's ops, so they say what a
// person expects the client to do, and the compiler and the assertions say
// whether it does it.

const rustCheck = `use vault::*;
use std::cell::RefCell;

// A transport that answers from memory, so the call path is exercised end to
// end without a socket.
struct Recording {
    seen: RefCell<(String, String, String)>,
    answer: String,
}

impl Transport for Recording {
    fn send(&self, method: &str, target: &str, body: Option<&[u8]>) -> Result<Reply, Error> {
        *self.seen.borrow_mut() = (
            method.to_string(),
            target.to_string(),
            String::from_utf8_lossy(body.unwrap_or(b"")).into_owned(),
        );
        Ok(Reply { status: 200, body: self.answer.clone().into_bytes() })
    }
}

fn recording(answer: &str) -> Recording {
    Recording { seen: RefCell::new(Default::default()), answer: answer.to_string() }
}

#[test]
fn a_path_parameter_is_escaped_into_the_address() {
    let t = recording(r#"{"name":"k","owner":{"org":"acme","kind":"user"}}"#);
    let got = {
        let c = Client::new(&t);
        c.vault_read(&VaultRead { name: "a b/c".into(), reveal: true }).unwrap()
    };
    let (method, target, body) = t.seen.into_inner();
    assert_eq!(method, "GET");
    assert_eq!(target, "/v1/secrets/a%20b%2Fc?reveal=true");
    assert_eq!(body, "");
    assert_eq!(got.name, "k");
    assert_eq!(got.owner.org, "acme");
}

#[test]
fn a_query_parameter_rides_the_url() {
    let t = recording(r#"{"secrets":[{"name":"one"}],"more":true}"#);
    let got = {
        let c = Client::new(&t);
        c.vault_list(&VaultList { org: "acme".into(), limit: 10 }).unwrap()
    };
    assert_eq!(t.seen.into_inner().1, "/v1/secrets?org=acme&limit=10");
    assert_eq!(got.secrets.len(), 1);
    assert!(got.more);
}

#[test]
fn a_body_op_sends_the_whole_value() {
    let t = recording(r#"{"ref":"r1","at":"2026-09-01T00:00:00Z"}"#);
    let mut input = Seal::default();
    input.name = "k".into();
    input.bytes = 42;
    input.weight = 1.5;
    input.rotate = true;
    input.tags = vec!["a".into(), "b".into()];
    input.counts = vec![1, 2];
    input.labels.insert("env".into(), "prod".into());
    input.owner.org = "acme".into();
    input.free = serde_json::json!({"x": 1});

    let got = {
        let c = Client::new(&t);
        c.vault_seal(&input).unwrap()
    };
    let (method, target, body) = t.seen.into_inner();
    assert_eq!(method, "POST");
    assert_eq!(target, "/v1/seal");
    let sent: serde_json::Value = serde_json::from_str(&body).unwrap();
    assert_eq!(sent["name"], "k");
    assert_eq!(sent["bytes"], 42);
    assert_eq!(sent["labels"]["env"], "prod");
    assert_eq!(sent["counts"][1], 2);
    assert_eq!(sent["free"]["x"], 1);
    assert_eq!(got.ref_, "r1");
}

#[test]
fn an_op_that_takes_nothing_takes_nothing() {
    let t = recording(r#"{"ready":true,"name":"vault"}"#);
    let got = {
        let c = Client::new(&t);
        c.vault_health().unwrap()
    };
    assert_eq!(t.seen.into_inner().1, "/v1/health");
    assert!(got.ready);
}

// An answer that leaves fields out is still an answer: the service is written
// in Go, where an absent field is the zero value.
#[test]
fn a_short_answer_reads_as_zero_values() {
    let t = recording(r#"{"name":"k"}"#);
    let got = {
        let c = Client::new(&t);
        c.vault_read(&VaultRead::default()).unwrap()
    };
    assert_eq!(got.name, "k");
    assert_eq!(got.owner.org, "");
    assert_eq!(got.at, "");
}

struct Refusing;

impl Transport for Refusing {
    fn send(&self, _: &str, _: &str, _: Option<&[u8]>) -> Result<Reply, Error> {
        Ok(Reply { status: 403, body: b"denied".to_vec() })
    }
}

#[test]
fn a_refusal_is_an_error_carrying_the_status() {
    match Client::new(Refusing).vault_health() {
        Err(Error::Status { status, body }) => {
            assert_eq!(status, 403);
            assert_eq!(body, "denied");
        }
        other => panic!("want a refusal, got {other:?}"),
    }
}
`

const cppCheck = `#include <vault/vault.hpp>

#include <cassert>
#include <string>

// A transport that answers from memory, so the call path is exercised end to
// end without a socket.
struct Recording : vault::Transport {
    std::string method, target, sent, answer;
    vault::Reply send(const std::string& m, const std::string& t,
                      const std::string* body) override {
        method = m;
        target = t;
        sent = body ? *body : std::string();
        return vault::Reply{200, answer};
    }
};

int main() {
    {   // A path parameter is escaped into the address.
        Recording t;
        t.answer = R"({"name":"k","owner":{"org":"acme","kind":"user"}})";
        vault::VaultRead in;
        in.name = "a b/c";
        in.reveal = true;
        const vault::Secret got = vault::Client(t).vault_read(in);
        assert(t.method == "GET");
        assert(t.target == "/v1/secrets/a%20b%2Fc?reveal=true");
        assert(t.sent.empty());
        assert(got.name == "k");
        assert(got.owner.org == "acme");
    }
    {   // A query parameter rides the URL.
        Recording t;
        t.answer = R"({"secrets":[{"name":"one"}],"more":true})";
        vault::VaultList in;
        in.org = "acme";
        in.limit = 10;
        const vault::Held got = vault::Client(t).vault_list(in);
        assert(t.target == "/v1/secrets?org=acme&limit=10");
        assert(got.secrets.size() == 1);
        assert(got.more);
    }
    {   // A body op sends the whole value.
        Recording t;
        t.answer = R"({"ref":"r1","at":"2026-09-01T00:00:00Z"})";
        vault::Seal in;
        in.name = "k";
        in.bytes = 42;
        in.weight = 1.5;
        in.rotate = true;
        in.tags = {"a", "b"};
        in.counts = {1, 2};
        in.labels["env"] = "prod";
        in.owner.org = "acme";
        in.free = nlohmann::json::object({{"x", 1}});
        const vault::Sealed got = vault::Client(t).vault_seal(in);
        assert(t.method == "POST");
        assert(t.target == "/v1/seal");
        const nlohmann::json sent = nlohmann::json::parse(t.sent);
        assert(sent["name"] == "k");
        assert(sent["bytes"] == 42);
        assert(sent["labels"]["env"] == "prod");
        assert(sent["counts"][1] == 2);
        assert(sent["free"]["x"] == 1);
        assert(got.ref == "r1");
    }
    {   // An op that takes nothing takes nothing.
        Recording t;
        t.answer = R"({"ready":true,"name":"vault"})";
        const vault::Ready got = vault::Client(t).vault_health();
        assert(t.target == "/v1/health");
        assert(got.ready);
    }
    {   // A short answer reads as zero values.
        Recording t;
        t.answer = R"({"name":"k"})";
        const vault::Secret got = vault::Client(t).vault_read(vault::VaultRead{});
        assert(got.name == "k");
        assert(got.owner.org.empty());
        assert(got.at.empty());
    }
    {   // A refusal is an exception carrying the status.
        struct Refusing : vault::Transport {
            vault::Reply send(const std::string&, const std::string&,
                              const std::string*) override {
                return vault::Reply{403, "denied"};
            }
        } refusing;
        try {
            vault::Client(refusing).vault_health();
            return 1;
        } catch (const vault::Refused& e) {
            assert(e.status() == 403);
            assert(e.body() == "denied");
        }
    }
    return 0;
}
`

const groveCheck = `#include <grove/grove.hpp>

#include <cassert>
#include <string>

struct Echo : grove::Transport {
    std::string sent;
    grove::Reply send(const std::string&, const std::string&,
                      const std::string* body) override {
        sent = body ? *body : std::string();
        return grove::Reply{200, sent};
    }
};

int main() {
    Echo t;
    grove::Tree in;
    in.name = "root";
    in.parent = std::make_shared<grove::Tree>();
    in.parent->name = "above";
    in.children.push_back(grove::Tree{});
    in.children[0].name = "below";
    in.grove = std::make_shared<grove::Forest>();

    const grove::Tree got = grove::Client(t).grove_plant(in);
    assert(got.name == "root");
    assert(got.parent && got.parent->name == "above");
    assert(got.children.size() == 1 && got.children[0].name == "below");
    // The mutual cycle is broken on one side; the other side is by value and
    // round-trips as an empty tree.
    assert(got.grove && got.grove->back.name.empty());
    return 0;
}
`
