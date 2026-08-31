---
title: The sqlrs engine API v0.1.0
language_tabs:
  - shell: Shell
  - http: HTTP
  - javascript: JavaScript
  - ruby: Ruby
  - python: Python
  - php: PHP
  - java: Java
  - go: Go
toc_footers: []
includes: []
search: true
highlight_theme: darkula
headingLevel: 2

---

<!-- Generator: Widdershins v4.0.1 -->

<h1 id="the-sqlrs-engine-api">The sqlrs engine API v0.1.0</h1>

> Scroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

sqlrs HTTP API (MVP).
This file is the design source of truth for the engine API. Local engine
deployments implement the local runtime subset. Shared/team/cloud
deployments additionally implement remote-only user and organization
management endpoints.

Base URLs:

* <a href="http://127.0.0.1:{port}">http://127.0.0.1:{port}</a>

    * **port** -  Default: 8080

License: <a href="https://www.apache.org/licenses/LICENSE-2.0.html">Apache-2.0</a>

# Authentication

- HTTP Authentication, scheme: bearer 

<h1 id="the-sqlrs-engine-api-health">health</h1>

## getHealth

<a id="opIdgetHealth"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/health \
  -H 'Accept: application/json'

```

```http
GET http://127.0.0.1:{port}/v1/health HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://127.0.0.1:{port}/v1/health',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/health',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.get('http://127.0.0.1:{port}/v1/health', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/health', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/health");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/health", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/health`

*Engine health check*

Returns engine status. No auth required.

> Example responses

> 200 Response

```json
{
  "ok": true,
  "version": "dev",
  "instanceId": "9f4d2d4b6c1a4a4ea2d39d1f7b0d8a21",
  "pid": 12345
}
```

<h3 id="gethealth-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[HealthResponse](#schemahealthresponse)|
|405|[Method Not Allowed](https://tools.ietf.org/html/rfc7231#section-6.5.5)|Method not allowed|None|

<aside class="success">
This operation does not require authentication
</aside>

<h1 id="the-sqlrs-engine-api-cache">cache</h1>

## getCacheStatus

<a id="opIdgetCacheStatus"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/cache/status \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/cache/status HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/cache/status',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/cache/status',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/cache/status', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/cache/status', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/cache/status");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/cache/status", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/cache/status`

*Get bounded-cache diagnostics*

Returns current bounded-cache occupancy, policy thresholds, pressure
reasons, and the latest completed eviction summary.

> Example responses

> 200 Response

```json
{
  "usage_bytes": 0,
  "configured_max_bytes": 0,
  "effective_max_bytes": 0,
  "reserve_bytes": 0,
  "high_watermark": 0,
  "low_watermark": 0,
  "min_state_age": "string",
  "store_total_bytes": 0,
  "store_free_bytes": 0,
  "state_count": 0,
  "reclaimable_bytes": 0,
  "blocked_count": 0,
  "pressure_reasons": [
    "string"
  ],
  "last_eviction": {
    "completed_at": "2019-08-24T14:15:22Z",
    "trigger": "string",
    "evicted_count": 0,
    "freed_bytes": 0,
    "blocked_count": 0,
    "reclaimable_bytes": 0,
    "usage_bytes_before": 0,
    "usage_bytes_after": 0,
    "free_bytes_before": 0,
    "free_bytes_after": 0
  }
}
```

<h3 id="getcachestatus-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[CacheStatus](#schemacachestatus)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|Internal error|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## explainPrepareCache

<a id="opIdexplainPrepareCache"></a>

> Code samples

```shell
# You can also use wget
curl -X POST http://127.0.0.1:{port}/v1/cache/explain/prepare \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
POST http://127.0.0.1:{port}/v1/cache/explain/prepare HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Accept: application/json

```

```javascript
const inputBody = '{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  }
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/cache/explain/prepare',
{
  method: 'POST',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.post 'http://127.0.0.1:{port}/v1/cache/explain/prepare',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.post('http://127.0.0.1:{port}/v1/cache/explain/prepare', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('POST','http://127.0.0.1:{port}/v1/cache/explain/prepare', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/cache/explain/prepare");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("POST");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("POST", "http://127.0.0.1:{port}/v1/cache/explain/prepare", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`POST /v1/cache/explain/prepare`

*Explain one prepare-oriented cache decision*

Computes the same final prepare signature and cache lookup the engine
would use for a bound single-stage prepare request, but does not create
a job, instance, or mutate cache state.

> Body parameter

```json
{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  }
}
```

<h3 id="explainpreparecache-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[CacheExplainPrepareRequest](#schemacacheexplainpreparerequest)|true|none|

> Example responses

> 200 Response

```json
{
  "decision": "hit",
  "reason_code": "string",
  "signature": "string",
  "matched_state_id": "string",
  "resolved_image_id": "string"
}
```

<h3 id="explainpreparecache-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[CacheExplainPrepareResponse](#schemacacheexplainprepareresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Source inputs missing; client may upload missing blobs or expand the source manifest and retry.|[SourceInputsMissingErrorResponse](#schemasourceinputsmissingerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|Internal error|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-source">source</h1>

## putSourceBlobSha256

<a id="opIdputSourceBlobSha256"></a>

> Code samples

```shell
# You can also use wget
curl -X PUT http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest} \
  -H 'Content-Type: application/octet-stream' \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
PUT http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest} HTTP/1.1
Host: 127.0.0.1
Content-Type: application/octet-stream
Accept: application/json

```

```javascript
const inputBody = 'string';
const headers = {
  'Content-Type':'application/octet-stream',
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest}',
{
  method: 'PUT',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/octet-stream',
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.put 'http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/octet-stream',
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.put('http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/octet-stream',
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('PUT','http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("PUT");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/octet-stream"},
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("PUT", "http://127.0.0.1:{port}/v1/source-blobs/sha256/{digest}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`PUT /v1/source-blobs/sha256/{digest}`

*Upload one remote source content blob*

Remote/shared deployments only. Stores one source-input content blob in
the authenticated actor's scoped content cache. The server verifies the
uploaded bytes by recomputing `sha256` and comparing it to `{digest}`.
The operation is idempotent: uploading content that is already present
succeeds without changing the source cache.

> Body parameter

```yaml
string

```

<h3 id="putsourceblobsha256-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|digest|path|string|true|Lowercase hex SHA-256 digest without the `sha256:` prefix.|
|body|body|string(binary)|true|none|

> Example responses

> 400 Response

```json
{
  "code": "string",
  "message": "string",
  "details": "string"
}
```

<h3 id="putsourceblobsha256-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|204|[No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5)|Blob stored or already present.|None|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid digest or upload request.|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Uploaded content does not match the requested digest.|[ErrorResponse](#schemaerrorresponse)|
|413|[Payload Too Large](https://tools.ietf.org/html/rfc7231#section-6.5.11)|Source blob exceeds the configured upload limit (`source_blob_too_large`).|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|Internal error|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-prepare">prepare</h1>

## listPrepareJobs

<a id="opIdlistPrepareJobs"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/prepare-jobs \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/prepare-jobs HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/prepare-jobs',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/prepare-jobs',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/prepare-jobs', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/prepare-jobs', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/prepare-jobs");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/prepare-jobs", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/prepare-jobs`

*List prepare jobs*

Returns prepare jobs.

<h3 id="listpreparejobs-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|job|query|string|false|Filter by job id prefix.|

> Example responses

> 200 Response

```json
[
  {
    "job_id": "string",
    "status": "queued",
    "prepare_kind": "string",
    "image_id": "string",
    "resolved_image_id": "string",
    "prepare_args_normalized": "string",
    "signature": "string",
    "plan_only": true,
    "created_at": "2019-08-24T14:15:22Z",
    "started_at": "2019-08-24T14:15:22Z",
    "finished_at": "2019-08-24T14:15:22Z"
  }
]
```

<h3 id="listpreparejobs-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid filter|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## createPrepareJob

<a id="opIdcreatePrepareJob"></a>

> Code samples

```shell
# You can also use wget
curl -X POST http://127.0.0.1:{port}/v1/prepare-jobs \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
POST http://127.0.0.1:{port}/v1/prepare-jobs HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Accept: application/json

```

```javascript
const inputBody = '{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  },
  "plan_only": true
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/prepare-jobs',
{
  method: 'POST',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.post 'http://127.0.0.1:{port}/v1/prepare-jobs',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.post('http://127.0.0.1:{port}/v1/prepare-jobs', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('POST','http://127.0.0.1:{port}/v1/prepare-jobs', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/prepare-jobs");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("POST");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("POST", "http://127.0.0.1:{port}/v1/prepare-jobs", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`POST /v1/prepare-jobs`

*Start a prepare job*

Starts a prepare job and returns a job reference.
The engine enforces deterministic defaults for `psql` (`-X` and
`-v ON_ERROR_STOP=1`) and rejects connection flags.
When `plan_only` is true, the job computes a plan and does not create
an instance; the plan tasks are returned in the job status.

> Body parameter

```json
{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  },
  "plan_only": true
}
```

<h3 id="createpreparejob-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[PrepareJobRequest](#schemapreparejobrequest)|true|none|

> Example responses

> 201 Response

```json
{
  "job_id": "string",
  "status_url": "string",
  "events_url": "string",
  "status": "queued"
}
```

<h3 id="createpreparejob-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[PrepareJobAccepted](#schemapreparejobaccepted)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Source inputs missing; client may upload missing blobs or expand the source manifest and retry.|[SourceInputsMissingErrorResponse](#schemasourceinputsmissingerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|Internal error|[ErrorResponse](#schemaerrorresponse)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|201|Location|string||Canonical job status URL.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## getPrepareJob

<a id="opIdgetPrepareJob"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/prepare-jobs/{jobId} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/prepare-jobs/{jobId} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/prepare-jobs/{jobId}`

*Get prepare job status*

Returns job status and result when available.

<h3 id="getpreparejob-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|jobId|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "job_id": "string",
  "status": "queued",
  "prepare_kind": "string",
  "image_id": "string",
  "plan_only": true,
  "prepare_args_normalized": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "started_at": "2019-08-24T14:15:22Z",
  "finished_at": "2019-08-24T14:15:22Z",
  "tasks": [
    {
      "task_id": "string",
      "type": "plan",
      "planner_kind": "string"
    }
  ],
  "result": {
    "dsn": "string",
    "instance_id": "string",
    "state_id": "string",
    "image_id": "string",
    "prepare_kind": "string",
    "prepare_args_normalized": "string"
  },
  "error": {
    "code": "string",
    "message": "string",
    "details": "string"
  }
}
```

<h3 id="getpreparejob-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[PrepareJobStatus](#schemapreparejobstatus)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid prefix filter|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## deletePrepareJob

<a id="opIddeletePrepareJob"></a>

> Code samples

```shell
# You can also use wget
curl -X DELETE http://127.0.0.1:{port}/v1/prepare-jobs/{jobId} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
DELETE http://127.0.0.1:{port}/v1/prepare-jobs/{jobId} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}',
{
  method: 'DELETE',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.delete 'http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.delete('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('DELETE','http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("DELETE");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("DELETE", "http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`DELETE /v1/prepare-jobs/{jobId}`

*Delete a prepare job*

Deletes a prepare job by id. This operation is idempotent.
By default, jobs with started tasks are blocked unless `force` is set.

<h3 id="deletepreparejob-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|jobId|path|string|true|none|
|force|query|boolean|false|Allow deletion of jobs with started tasks.|
|dry_run|query|boolean|false|Return the deletion tree without making changes.|

#### Detailed descriptions

**dry_run**: Return the deletion tree without making changes.
When true, the response is always 200 and `outcome` indicates whether
deletion would succeed.

> Example responses

> 200 Response

```json
{
  "dry_run": true,
  "outcome": "deleted",
  "root": {
    "kind": "state",
    "id": "string",
    "connections": 0,
    "runtime_id": "string",
    "blocked": "active_connections",
    "children": [
      {}
    ]
  }
}
```

<h3 id="deletepreparejob-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK (deleted or dry-run result)|[DeleteResult](#schemadeleteresult)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Blocked by safety rules (dry_run=false only)|[DeleteResult](#schemadeleteresult)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## cancelPrepareJob

<a id="opIdcancelPrepareJob"></a>

> Code samples

```shell
# You can also use wget
curl -X POST http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
POST http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel',
{
  method: 'POST',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.post 'http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.post('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('POST','http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("POST");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("POST", "http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/cancel", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`POST /v1/prepare-jobs/{jobId}/cancel`

*Cancel a prepare job*

Requests cancellation for a queued or running prepare job.
The operation is idempotent:
- `202 Accepted` when cancellation is requested for `queued` or `running`.
- `200 OK` when the job is already terminal (`succeeded`, `failed`).

<h3 id="cancelpreparejob-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|jobId|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "job_id": "string",
  "status": "queued",
  "prepare_kind": "string",
  "image_id": "string",
  "plan_only": true,
  "prepare_args_normalized": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "started_at": "2019-08-24T14:15:22Z",
  "finished_at": "2019-08-24T14:15:22Z",
  "tasks": [
    {
      "task_id": "string",
      "type": "plan",
      "planner_kind": "string"
    }
  ],
  "result": {
    "dsn": "string",
    "instance_id": "string",
    "state_id": "string",
    "image_id": "string",
    "prepare_kind": "string",
    "prepare_args_normalized": "string"
  },
  "error": {
    "code": "string",
    "message": "string",
    "details": "string"
  }
}
```

<h3 id="cancelpreparejob-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Already terminal|[PrepareJobStatus](#schemapreparejobstatus)|
|202|[Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3)|Cancellation requested|[PrepareJobStatus](#schemapreparejobstatus)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## streamPrepareJobEvents

<a id="opIdstreamPrepareJobEvents"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events \
  -H 'Accept: application/x-ndjson' \
  -H 'Range: string' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events HTTP/1.1
Host: 127.0.0.1
Accept: application/x-ndjson
Range: string

```

```javascript

const headers = {
  'Accept':'application/x-ndjson',
  'Range':'string',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/x-ndjson',
  'Range' => 'string',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/x-ndjson',
  'Range': 'string',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/x-ndjson',
    'Range' => 'string',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/x-ndjson"},
        "Range": []string{"string"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/prepare-jobs/{jobId}/events", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/prepare-jobs/{jobId}/events`

*Stream prepare job events*

Streams job events as NDJSON.
Clients may request a partial stream using an events-based range; if the
server does not honor the range request, it returns a full 200 response.

<h3 id="streampreparejobevents-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|jobId|path|string|true|none|
|Range|header|string|false|Optional events-based range request, e.g. `events=10-` or `events=10-24`.|

#### Detailed descriptions

**Range**: Optional events-based range request, e.g. `events=10-` or `events=10-24`.
When supported, the server responds with 206 and `Content-Range: events`.

> Example responses

> 200 Response

> 400 Response

```json
{
  "code": "string",
  "message": "string",
  "details": "string"
}
```

<h3 id="streampreparejobevents-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[PrepareJobEvent](#schemapreparejobevent)|
|206|[Partial Content](https://tools.ietf.org/html/rfc7233#section-4.1)|Partial Content (events range)|[PrepareJobEvent](#schemapreparejobevent)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid prefix filter|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|None|
|416|[Range Not Satisfiable](https://tools.ietf.org/html/rfc7233#section-4.4)|Requested range not satisfiable|[ErrorResponse](#schemaerrorresponse)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|Accept-Ranges|string||Range unit support (e.g. `events`).|
|206|Content-Range|string||Events range returned, e.g. `events 10-24/100` or `events 10-*/100`.
|
|206|Accept-Ranges|string||Range unit support (e.g. `events`).|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-tasks">tasks</h1>

## listTasks

<a id="opIdlistTasks"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/tasks \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/tasks HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/tasks',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/tasks',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/tasks', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/tasks', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/tasks");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/tasks", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/tasks`

*List tasks*

Returns task queue entries across jobs.

<h3 id="listtasks-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|job|query|string|false|Filter by job id (exact match).|

> Example responses

> 200 Response

```json
[
  {
    "task_id": "string",
    "job_id": "string",
    "type": "plan",
    "status": "queued",
    "planner_kind": "string",
    "input": {
      "kind": "image",
      "id": "string"
    },
    "task_hash": "string",
    "output_state_id": "string",
    "cached": true,
    "instance_mode": "ephemeral",
    "image_id": "string",
    "resolved_image_id": "string",
    "args_summary": "string",
    "changeset_id": "string",
    "changeset_author": "string",
    "changeset_path": "string"
  }
]
```

<h3 id="listtasks-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid filter|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-config">config</h1>

## getConfig

<a id="opIdgetConfig"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/config \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/config HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/config',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/config',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/config', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/config', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/config");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/config", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/config`

*Get server config*

Returns the server configuration.
When `path` is provided, returns the value at that path.
When `effective` is true, defaults are applied before returning the value.

<h3 id="getconfig-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|path|query|string|false|JS-style path (e.g. `orchestrator.jobs.maxIdentical`).|
|effective|query|boolean|false|Return value after applying defaults.|

> Example responses

> 200 Response

```json
{
  "path": "string",
  "value": {}
}
```

<h3 id="getconfig-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Config value or full config object.|Inline|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid path|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Path not found|None|

<h3 id="getconfig-responseschema">Response Schema</h3>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## setConfig

<a id="opIdsetConfig"></a>

> Code samples

```shell
# You can also use wget
curl -X PATCH http://127.0.0.1:{port}/v1/config \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
PATCH http://127.0.0.1:{port}/v1/config HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Accept: application/json

```

```javascript
const inputBody = '{
  "path": "string",
  "value": {}
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/config',
{
  method: 'PATCH',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.patch 'http://127.0.0.1:{port}/v1/config',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.patch('http://127.0.0.1:{port}/v1/config', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('PATCH','http://127.0.0.1:{port}/v1/config', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/config");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("PATCH");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("PATCH", "http://127.0.0.1:{port}/v1/config", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`PATCH /v1/config`

*Set config value*

Sets a configuration value by path.
The value is validated against the server schema and semantic rules.

> Body parameter

```json
{
  "path": "string",
  "value": {}
}
```

<h3 id="setconfig-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[ConfigSetRequest](#schemaconfigsetrequest)|true|none|

> Example responses

> 200 Response

```json
{
  "path": "string",
  "value": {}
}
```

<h3 id="setconfig-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Updated config value.|[ConfigValue](#schemaconfigvalue)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid value|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## removeConfig

<a id="opIdremoveConfig"></a>

> Code samples

```shell
# You can also use wget
curl -X DELETE http://127.0.0.1:{port}/v1/config?path=string \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
DELETE http://127.0.0.1:{port}/v1/config?path=string HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/config?path=string',
{
  method: 'DELETE',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.delete 'http://127.0.0.1:{port}/v1/config',
  params: {
  'path' => 'string'
}, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.delete('http://127.0.0.1:{port}/v1/config', params={
  'path': 'string'
}, headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('DELETE','http://127.0.0.1:{port}/v1/config', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/config?path=string");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("DELETE");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("DELETE", "http://127.0.0.1:{port}/v1/config", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`DELETE /v1/config`

*Remove config value*

Removes a configuration value by path.

<h3 id="removeconfig-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|path|query|string|true|JS-style path (e.g. `orchestrator.jobs.maxIdentical`).|
|effective|query|boolean|false|Return value after applying defaults.|

> Example responses

> 200 Response

```json
{
  "path": "string",
  "value": {}
}
```

<h3 id="removeconfig-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Updated config value.|[ConfigValue](#schemaconfigvalue)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid path|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Path not found|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## getConfigSchema

<a id="opIdgetConfigSchema"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/config/schema \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/config/schema HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/config/schema',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/config/schema',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/config/schema', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/config/schema', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/config/schema");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/config/schema", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/config/schema`

*Get config schema*

Returns the JSON Schema for server config validation.

> Example responses

> 200 Response

```json
{}
```

<h3 id="getconfigschema-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|JSON Schema|[ConfigSchema](#schemaconfigschema)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-users">users</h1>

## getCurrentUser

<a id="opIdgetCurrentUser"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/users/me \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/users/me HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/users/me',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/users/me',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/users/me', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/users/me', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/users/me");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/users/me", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/users/me`

*Get current user profile*

Remote/shared deployments only. Returns the current user profile,
linked external identities, and organization memberships.

> Example responses

> 200 Response

```json
{
  "status": "created",
  "user": {
    "id": "string",
    "display_name": "string",
    "email": "user@example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "identities": [
    {
      "provider": "oidc",
      "issuer": "string",
      "subject": "string"
    }
  ],
  "memberships": [
    {
      "organization": {
        "id": "string",
        "slug": "string",
        "display_name": "string",
        "created_at": "2019-08-24T14:15:22Z",
        "updated_at": "2019-08-24T14:15:22Z"
      },
      "membership": {
        "user_id": "string",
        "organization_id": "string",
        "role": "admin",
        "created_at": "2019-08-24T14:15:22Z"
      }
    }
  ]
}
```

<h3 id="getcurrentuser-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[UserProfileResult](#schemauserprofileresult)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|User profile not found|[ErrorResponse](#schemaerrorresponse)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Current entity tag for conditional updates.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## putCurrentUser

<a id="opIdputCurrentUser"></a>

> Code samples

```shell
# You can also use wget
curl -X PUT http://127.0.0.1:{port}/v1/users/me \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'If-None-Match: string' \
  -H 'If-Match: string' \
  -H 'Authorization: Bearer {access-token}'

```

```http
PUT http://127.0.0.1:{port}/v1/users/me HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Accept: application/json
If-None-Match: string
If-Match: string

```

```javascript
const inputBody = '{
  "display_name": "string",
  "email": "user@example.com"
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json',
  'If-None-Match':'string',
  'If-Match':'string',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/users/me',
{
  method: 'PUT',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json',
  'If-None-Match' => 'string',
  'If-Match' => 'string',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.put 'http://127.0.0.1:{port}/v1/users/me',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'If-None-Match': 'string',
  'If-Match': 'string',
  'Authorization': 'Bearer {access-token}'
}

r = requests.put('http://127.0.0.1:{port}/v1/users/me', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
    'If-None-Match' => 'string',
    'If-Match' => 'string',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('PUT','http://127.0.0.1:{port}/v1/users/me', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/users/me");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("PUT");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
        "If-None-Match": []string{"string"},
        "If-Match": []string{"string"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("PUT", "http://127.0.0.1:{port}/v1/users/me", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`PUT /v1/users/me`

*Create or update current user profile*

Remote/shared deployments only.

Conditional PUT for the current authenticated external identity. The
server derives `provider + issuer + subject` from validated bearer-token
claims.

Clients must send exactly one HTTP precondition:

- `If-None-Match: *` for self-registration create-only behavior.
- `If-Match: <etag>` for update-only changes to an existing profile.

Missing preconditions are rejected with `428 Precondition Required`.
Sending both preconditions is rejected with `400`. If
self-registration is disabled and no current user profile exists yet,
create-only requests are rejected with `403`.

> Body parameter

```json
{
  "display_name": "string",
  "email": "user@example.com"
}
```

<h3 id="putcurrentuser-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|If-None-Match|header|string|false|Use `*` for self-registration create-only behavior.|
|If-Match|header|string|false|Use the current ETag for update-only changes.|
|body|body|[UserProfileWriteRequest](#schemauserprofilewriterequest)|true|none|

> Example responses

> 200 Response

```json
{
  "status": "created",
  "user": {
    "id": "string",
    "display_name": "string",
    "email": "user@example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "identities": [
    {
      "provider": "oidc",
      "issuer": "string",
      "subject": "string"
    }
  ],
  "memberships": [
    {
      "organization": {
        "id": "string",
        "slug": "string",
        "display_name": "string",
        "created_at": "2019-08-24T14:15:22Z",
        "updated_at": "2019-08-24T14:15:22Z"
      },
      "membership": {
        "user_id": "string",
        "organization_id": "string",
        "role": "admin",
        "created_at": "2019-08-24T14:15:22Z"
      }
    }
  ]
}
```

<h3 id="putcurrentuser-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Current user profile updated|[UserProfileResult](#schemauserprofileresult)|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Current user profile created|[UserProfileResult](#schemauserprofileresult)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input or conflicting preconditions|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Self-registration disabled or forbidden|[ErrorResponse](#schemaerrorresponse)|
|412|[Precondition Failed](https://tools.ietf.org/html/rfc7232#section-4.2)|Precondition failed|[ErrorResponse](#schemaerrorresponse)|
|428|[Precondition Required](https://tools.ietf.org/html/rfc6585#section-3)|Precondition required|[ErrorResponse](#schemaerrorresponse)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||New entity tag for the updated current user profile.|
|201|ETag|string||Entity tag for the created current user profile.|
|201|Location|string||Current user resource URL.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## getUserByIdentity

<a id="opIdgetUserByIdentity"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/users/by-identity',
  params: {
  'provider' => 'string',
'issuer' => 'string',
'subject' => 'string'
}, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/users/by-identity', params={
  'provider': 'oidc',  'issuer': 'string',  'subject': 'string'
}, headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/users/by-identity', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/users/by-identity", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/users/by-identity`

*Get a user by external identity*

Remote/shared deployments only.

Administrator operation that resolves a user profile by the natural
external identity key `provider + issuer + subject`.

<h3 id="getuserbyidentity-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|provider|query|string|true|External identity provider kind.|
|issuer|query|string|true|OAuth/OIDC issuer identifier.|
|subject|query|string|true|Provider-local subject identifier.|

> Example responses

> 200 Response

```json
{
  "status": "created",
  "user": {
    "id": "string",
    "display_name": "string",
    "email": "user@example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "identities": [
    {
      "provider": "oidc",
      "issuer": "string",
      "subject": "string"
    }
  ],
  "memberships": [
    {
      "organization": {
        "id": "string",
        "slug": "string",
        "display_name": "string",
        "created_at": "2019-08-24T14:15:22Z",
        "updated_at": "2019-08-24T14:15:22Z"
      },
      "membership": {
        "user_id": "string",
        "organization_id": "string",
        "role": "admin",
        "created_at": "2019-08-24T14:15:22Z"
      }
    }
  ]
}
```

<h3 id="getuserbyidentity-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[UserProfileResult](#schemauserprofileresult)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Administrator permission required|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|External identity not linked|[ErrorResponse](#schemaerrorresponse)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Current entity tag for conditional updates.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## putUserByIdentity

<a id="opIdputUserByIdentity"></a>

> Code samples

```shell
# You can also use wget
curl -X PUT http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'If-None-Match: string' \
  -H 'If-Match: string' \
  -H 'Authorization: Bearer {access-token}'

```

```http
PUT http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Accept: application/json
If-None-Match: string
If-Match: string

```

```javascript
const inputBody = '{
  "display_name": "string",
  "email": "user@example.com"
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json',
  'If-None-Match':'string',
  'If-Match':'string',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string',
{
  method: 'PUT',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json',
  'If-None-Match' => 'string',
  'If-Match' => 'string',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.put 'http://127.0.0.1:{port}/v1/users/by-identity',
  params: {
  'provider' => 'string',
'issuer' => 'string',
'subject' => 'string'
}, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'If-None-Match': 'string',
  'If-Match': 'string',
  'Authorization': 'Bearer {access-token}'
}

r = requests.put('http://127.0.0.1:{port}/v1/users/by-identity', params={
  'provider': 'oidc',  'issuer': 'string',  'subject': 'string'
}, headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
    'If-None-Match' => 'string',
    'If-Match' => 'string',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('PUT','http://127.0.0.1:{port}/v1/users/by-identity', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/users/by-identity?provider=oidc&issuer=string&subject=string");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("PUT");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
        "If-None-Match": []string{"string"},
        "If-Match": []string{"string"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("PUT", "http://127.0.0.1:{port}/v1/users/by-identity", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`PUT /v1/users/by-identity`

*Create or update a user by external identity*

Remote/shared deployments only.

Administrator operation keyed by the natural external identity
`provider + issuer + subject`.

Clients must send exactly one HTTP precondition:

- `If-None-Match: *` for create-only provisioning.
- `If-Match: <etag>` for update-only changes to an existing profile.

Missing preconditions are rejected with `428 Precondition Required`.
Sending both preconditions is rejected with `400`. The server enforces
external identity uniqueness over `provider + issuer + subject`.

> Body parameter

```json
{
  "display_name": "string",
  "email": "user@example.com"
}
```

<h3 id="putuserbyidentity-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|provider|query|string|true|External identity provider kind.|
|issuer|query|string|true|OAuth/OIDC issuer identifier.|
|subject|query|string|true|Provider-local subject identifier.|
|If-None-Match|header|string|false|Use `*` for create-only provisioning.|
|If-Match|header|string|false|Use the current ETag for update-only changes.|
|body|body|[UserProfileWriteRequest](#schemauserprofilewriterequest)|true|none|

> Example responses

> 200 Response

```json
{
  "status": "created",
  "user": {
    "id": "string",
    "display_name": "string",
    "email": "user@example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "identities": [
    {
      "provider": "oidc",
      "issuer": "string",
      "subject": "string"
    }
  ],
  "memberships": [
    {
      "organization": {
        "id": "string",
        "slug": "string",
        "display_name": "string",
        "created_at": "2019-08-24T14:15:22Z",
        "updated_at": "2019-08-24T14:15:22Z"
      },
      "membership": {
        "user_id": "string",
        "organization_id": "string",
        "role": "admin",
        "created_at": "2019-08-24T14:15:22Z"
      }
    }
  ]
}
```

<h3 id="putuserbyidentity-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|User profile updated|[UserProfileResult](#schemauserprofileresult)|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|User profile created|[UserProfileResult](#schemauserprofileresult)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Administrator permission required|[ErrorResponse](#schemaerrorresponse)|
|412|[Precondition Failed](https://tools.ietf.org/html/rfc7232#section-4.2)|Precondition failed (identity already linked or stale ETag)|[ErrorResponse](#schemaerrorresponse)|
|428|[Precondition Required](https://tools.ietf.org/html/rfc6585#section-3)|Precondition required|[ErrorResponse](#schemaerrorresponse)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||New entity tag for the updated user profile.|
|201|ETag|string||Entity tag for the created user profile.|
|201|Location|string||Identity-keyed user resource URL.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-organizations">organizations</h1>

## listOrganizations

<a id="opIdlistOrganizations"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/organizations \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/organizations HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/organizations',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/organizations',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/organizations', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/organizations', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/organizations");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/organizations", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/organizations`

*List current user's organizations*

Remote/shared deployments only. Returns organizations visible to the
current authenticated user.

> Example responses

> 200 Response

```json
[
  {
    "organization": {
      "id": "string",
      "slug": "string",
      "display_name": "string",
      "created_at": "2019-08-24T14:15:22Z",
      "updated_at": "2019-08-24T14:15:22Z"
    },
    "membership": {
      "user_id": "string",
      "organization_id": "string",
      "role": "admin",
      "created_at": "2019-08-24T14:15:22Z"
    }
  }
]
```

<h3 id="listorganizations-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|Inline|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|User profile not found|[ErrorResponse](#schemaerrorresponse)|

<h3 id="listorganizations-responseschema">Response Schema</h3>

Status Code **200**

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[OrganizationMembershipView](#schemaorganizationmembershipview)]|false|none|none|
|» organization|[Organization](#schemaorganization)|true|none|none|
|»» id|string|true|none|Stable sqlrs organization id.|
|»» slug|string|true|none|Stable human-readable organization reference.|
|»» display_name|string|true|none|none|
|»» created_at|string(date-time)|true|none|none|
|»» updated_at|string(date-time)|true|none|none|
|» membership|[OrganizationMembership](#schemaorganizationmembership)|true|none|none|
|»» user_id|string|true|none|none|
|»» organization_id|string|true|none|none|
|»» role|string|true|none|none|
|»» created_at|string(date-time)|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|role|admin|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## createOrganization

<a id="opIdcreateOrganization"></a>

> Code samples

```shell
# You can also use wget
curl -X POST http://127.0.0.1:{port}/v1/organizations \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
POST http://127.0.0.1:{port}/v1/organizations HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Accept: application/json

```

```javascript
const inputBody = '{
  "slug": "string",
  "display_name": "string"
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/organizations',
{
  method: 'POST',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.post 'http://127.0.0.1:{port}/v1/organizations',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.post('http://127.0.0.1:{port}/v1/organizations', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('POST','http://127.0.0.1:{port}/v1/organizations', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/organizations");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("POST");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("POST", "http://127.0.0.1:{port}/v1/organizations", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`POST /v1/organizations`

*Create an organization*

Remote/shared deployments only. Creates a new organization and makes
the current registered user its `admin`. In the first slice, the server
rejects organization creation for users that already belong to an
organization.

> Body parameter

```json
{
  "slug": "string",
  "display_name": "string"
}
```

<h3 id="createorganization-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[OrganizationCreateRequest](#schemaorganizationcreaterequest)|true|none|

> Example responses

> 201 Response

```json
{
  "organization": {
    "id": "string",
    "slug": "string",
    "display_name": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "membership": {
    "user_id": "string",
    "organization_id": "string",
    "role": "admin",
    "created_at": "2019-08-24T14:15:22Z"
  }
}
```

<h3 id="createorganization-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Organization created|[OrganizationCreateResponse](#schemaorganizationcreateresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|User profile not found|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Slug already taken or user is already attached to an organization|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## getOrganization

<a id="opIdgetOrganization"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/organizations/{orgRef} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/organizations/{orgRef} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/organizations/{orgRef}',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/organizations/{orgRef}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/organizations/{orgRef}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/organizations/{orgRef}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/organizations/{orgRef}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/organizations/{orgRef}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/organizations/{orgRef}`

*Get an organization visible to current user*

Remote/shared deployments only. Returns an organization by id or slug
when it is visible to the current authenticated user. Non-members
receive `404`.

<h3 id="getorganization-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|orgRef|path|string|true|Organization id or slug.|

> Example responses

> 200 Response

```json
{
  "organization": {
    "id": "string",
    "slug": "string",
    "display_name": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "membership": {
    "user_id": "string",
    "organization_id": "string",
    "role": "admin",
    "created_at": "2019-08-24T14:15:22Z"
  }
}
```

<h3 id="getorganization-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[OrganizationMembershipView](#schemaorganizationmembershipview)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|User profile or visible organization not found|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-names">names</h1>

## listNames

<a id="opIdlistNames"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/names \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/names HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/names',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/names',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/names', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/names', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/names");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/names", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/names`

*List names*

Returns name bindings.

<h3 id="listnames-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|instance|query|string|false|Filter by instance id.|
|state|query|string|false|Filter by state id.|
|image|query|string|false|Filter by base image id.|

> Example responses

> 200 Response

```json
[
  {
    "name": "string",
    "instance_id": "string",
    "image_id": "string",
    "state_id": "string",
    "state_fingerprint": "string",
    "status": "active",
    "last_used_at": "2019-08-24T14:15:22Z"
  }
]
```

<h3 id="listnames-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## getName

<a id="opIdgetName"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/names/{name} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/names/{name} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/names/{name}',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/names/{name}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/names/{name}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/names/{name}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/names/{name}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/names/{name}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/names/{name}`

*Get a name binding*

Returns a single name binding.

<h3 id="getname-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "name": "string",
  "instance_id": "string",
  "image_id": "string",
  "state_id": "string",
  "state_fingerprint": "string",
  "status": "active",
  "last_used_at": "2019-08-24T14:15:22Z"
}
```

<h3 id="getname-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[NameEntry](#schemanameentry)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-instances">instances</h1>

## listInstances

<a id="opIdlistInstances"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/instances \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/instances HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/instances',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/instances',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/instances', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/instances', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/instances");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/instances", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/instances`

*List instances*

Returns instances.

<h3 id="listinstances-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id_prefix|query|string|false|Case-insensitive hex id prefix (min 8 chars).|
|state|query|string|false|Filter by state id.|
|image|query|string|false|Filter by base image id.|

> Example responses

> 200 Response

```json
[
  {
    "instance_id": "string",
    "image_id": "string",
    "state_id": "string",
    "name": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "expires_at": "2019-08-24T14:15:22Z",
    "status": "active"
  }
]
```

<h3 id="listinstances-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid filter|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## getInstance

<a id="opIdgetInstance"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/instances/{instanceId} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/instances/{instanceId} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/instances/{instanceId}',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/instances/{instanceId}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/instances/{instanceId}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/instances/{instanceId}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/instances/{instanceId}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/instances/{instanceId}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/instances/{instanceId}`

*Get an instance*

Returns a single instance by id. If the path segment does not match the
instance id format, it is treated as a name alias. If it matches the id
format, the engine first attempts id lookup, then falls back to name.
When resolved by name, the response is a temporary redirect to the
canonical id-based URL.

<h3 id="getinstance-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|instanceId|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "instance_id": "string",
  "image_id": "string",
  "state_id": "string",
  "name": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "expires_at": "2019-08-24T14:15:22Z",
  "status": "active"
}
```

<h3 id="getinstance-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[InstanceEntry](#schemainstanceentry)|
|307|[Temporary Redirect](https://tools.ietf.org/html/rfc7231#section-6.4.7)|Temporary redirect to canonical instance id URL|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|None|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|307|Location|string||Canonical instance URL.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## deleteInstance

<a id="opIddeleteInstance"></a>

> Code samples

```shell
# You can also use wget
curl -X DELETE http://127.0.0.1:{port}/v1/instances/{instanceId} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
DELETE http://127.0.0.1:{port}/v1/instances/{instanceId} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/instances/{instanceId}',
{
  method: 'DELETE',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.delete 'http://127.0.0.1:{port}/v1/instances/{instanceId}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.delete('http://127.0.0.1:{port}/v1/instances/{instanceId}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('DELETE','http://127.0.0.1:{port}/v1/instances/{instanceId}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/instances/{instanceId}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("DELETE");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("DELETE", "http://127.0.0.1:{port}/v1/instances/{instanceId}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`DELETE /v1/instances/{instanceId}`

*Delete an instance*

Deletes a single instance by id. This operation is idempotent.
The response includes a deletion tree with blocked reasons when removal
is not possible.

<h3 id="deleteinstance-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|instanceId|path|string|true|none|
|force|query|boolean|false|Ignore active connections.|
|dry_run|query|boolean|false|Return the deletion tree without making changes.|

#### Detailed descriptions

**dry_run**: Return the deletion tree without making changes.
When true, the response is always 200 and `outcome` indicates whether
deletion would succeed.

> Example responses

> 200 Response

```json
{
  "dry_run": true,
  "outcome": "deleted",
  "root": {
    "kind": "state",
    "id": "string",
    "connections": 0,
    "runtime_id": "string",
    "blocked": "active_connections",
    "children": [
      {}
    ]
  }
}
```

<h3 id="deleteinstance-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK (deleted or dry-run result)|[DeleteResult](#schemadeleteresult)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Blocked by safety rules (dry_run=false only)|[DeleteResult](#schemadeleteresult)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-run">run</h1>

## runCommand

<a id="opIdrunCommand"></a>

> Code samples

```shell
# You can also use wget
curl -X POST http://127.0.0.1:{port}/v1/runs \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/x-ndjson' \
  -H 'Authorization: Bearer {access-token}'

```

```http
POST http://127.0.0.1:{port}/v1/runs HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
Accept: application/x-ndjson

```

```javascript
const inputBody = '{
  "instance_ref": "string",
  "kind": "psql",
  "command": "string",
  "args": [
    "string"
  ],
  "stdin": "string",
  "steps": [
    {
      "args": [
        "string"
      ],
      "stdin": "string"
    }
  ]
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/x-ndjson',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/runs',
{
  method: 'POST',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/x-ndjson',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.post 'http://127.0.0.1:{port}/v1/runs',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/x-ndjson',
  'Authorization': 'Bearer {access-token}'
}

r = requests.post('http://127.0.0.1:{port}/v1/runs', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/x-ndjson',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('POST','http://127.0.0.1:{port}/v1/runs', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/runs");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("POST");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/x-ndjson"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("POST", "http://127.0.0.1:{port}/v1/runs", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`POST /v1/runs`

*Run a command against an instance*

Executes a run kind command against an existing instance and streams
stdout/stderr/exit events. The stream may also include log events for
instance recovery when a missing container is recreated. If `command`
is omitted, the default command for the selected kind is used.

> Body parameter

```json
{
  "instance_ref": "string",
  "kind": "psql",
  "command": "string",
  "args": [
    "string"
  ],
  "stdin": "string",
  "steps": [
    {
      "args": [
        "string"
      ],
      "stdin": "string"
    }
  ]
}
```

<h3 id="runcommand-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[RunRequest](#schemarunrequest)|true|none|

> Example responses

> 200 Response

> 400 Response

```json
{
  "code": "string",
  "message": "string",
  "details": "string"
}
```

<h3 id="runcommand-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Streamed run output|[RunEvent](#schemarunevent)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Instance not found or expired|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Conflicting run arguments|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

<h1 id="the-sqlrs-engine-api-states">states</h1>

## listStates

<a id="opIdlistStates"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/states \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/states HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/states',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/states',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/states', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/states', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/states");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/states", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/states`

*List states*

Returns states.

<h3 id="liststates-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id_prefix|query|string|false|Case-insensitive hex id prefix (min 8 chars).|
|kind|query|string|false|Filter by prepare kind.|
|image|query|string|false|Filter by base image id.|

> Example responses

> 200 Response

```json
[
  {
    "state_id": "string",
    "parent_state_id": "string",
    "image_id": "string",
    "prepare_kind": "string",
    "prepare_args_normalized": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "size_bytes": 0,
    "last_used_at": "2019-08-24T14:15:22Z",
    "use_count": 0,
    "min_retention_until": "2019-08-24T14:15:22Z",
    "refcount": 0
  }
]
```

<h3 id="liststates-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid filter|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## getState

<a id="opIdgetState"></a>

> Code samples

```shell
# You can also use wget
curl -X GET http://127.0.0.1:{port}/v1/states/{stateId} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
GET http://127.0.0.1:{port}/v1/states/{stateId} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/states/{stateId}',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.get 'http://127.0.0.1:{port}/v1/states/{stateId}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.get('http://127.0.0.1:{port}/v1/states/{stateId}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://127.0.0.1:{port}/v1/states/{stateId}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/states/{stateId}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://127.0.0.1:{port}/v1/states/{stateId}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /v1/states/{stateId}`

*Get a state*

Returns a single state.

<h3 id="getstate-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|stateId|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "state_id": "string",
  "parent_state_id": "string",
  "image_id": "string",
  "prepare_kind": "string",
  "prepare_args_normalized": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "size_bytes": 0,
  "last_used_at": "2019-08-24T14:15:22Z",
  "use_count": 0,
  "min_retention_until": "2019-08-24T14:15:22Z",
  "refcount": 0
}
```

<h3 id="getstate-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[StateEntry](#schemastateentry)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

## deleteState

<a id="opIddeleteState"></a>

> Code samples

```shell
# You can also use wget
curl -X DELETE http://127.0.0.1:{port}/v1/states/{stateId} \
  -H 'Accept: application/json' \
  -H 'Authorization: Bearer {access-token}'

```

```http
DELETE http://127.0.0.1:{port}/v1/states/{stateId} HTTP/1.1
Host: 127.0.0.1
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json',
  'Authorization':'Bearer {access-token}'
};

fetch('http://127.0.0.1:{port}/v1/states/{stateId}',
{
  method: 'DELETE',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'Authorization' => 'Bearer {access-token}'
}

result = RestClient.delete 'http://127.0.0.1:{port}/v1/states/{stateId}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'Authorization': 'Bearer {access-token}'
}

r = requests.delete('http://127.0.0.1:{port}/v1/states/{stateId}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'Authorization' => 'Bearer {access-token}',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('DELETE','http://127.0.0.1:{port}/v1/states/{stateId}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://127.0.0.1:{port}/v1/states/{stateId}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("DELETE");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "Authorization": []string{"Bearer {access-token}"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("DELETE", "http://127.0.0.1:{port}/v1/states/{stateId}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`DELETE /v1/states/{stateId}`

*Delete a state*

Deletes a single state by id. This operation is idempotent.
By default, the state must have no descendants. When `recurse` is set,
the engine returns a full deletion tree and only deletes when all nodes
are allowed. Partial deletion is not performed.

<h3 id="deletestate-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|stateId|path|string|true|none|
|recurse|query|boolean|false|Allow removal of descendant states and instances.|
|force|query|boolean|false|Ignore active connections.|
|dry_run|query|boolean|false|Return the deletion tree without making changes.|

#### Detailed descriptions

**dry_run**: Return the deletion tree without making changes.
When true, the response is always 200 and `outcome` indicates whether
deletion would succeed.

> Example responses

> 200 Response

```json
{
  "dry_run": true,
  "outcome": "deleted",
  "root": {
    "kind": "state",
    "id": "string",
    "connections": 0,
    "runtime_id": "string",
    "blocked": "active_connections",
    "children": [
      {}
    ]
  }
}
```

<h3 id="deletestate-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK (deleted or dry-run result)|[DeleteResult](#schemadeleteresult)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Invalid input|[ErrorResponse](#schemaerrorresponse)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Unauthorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|Not found|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Blocked by safety rules (dry_run=false only)|[DeleteResult](#schemadeleteresult)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
bearerAuth
</aside>

# Schemas

<h2 id="tocS_HealthResponse">HealthResponse</h2>
<!-- backwards compatibility -->
<a id="schemahealthresponse"></a>
<a id="schema_HealthResponse"></a>
<a id="tocShealthresponse"></a>
<a id="tocshealthresponse"></a>

```json
{
  "ok": true,
  "version": "dev",
  "instanceId": "9f4d2d4b6c1a4a4ea2d39d1f7b0d8a21",
  "pid": 12345
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|ok|boolean|true|none|True if the engine is healthy.|
|version|string|true|none|Engine version string.|
|instanceId|string|true|none|Unique engine instance identifier.|
|pid|integer(int32)|true|none|Engine process id.|

<h2 id="tocS_CacheEvictionSummary">CacheEvictionSummary</h2>
<!-- backwards compatibility -->
<a id="schemacacheevictionsummary"></a>
<a id="schema_CacheEvictionSummary"></a>
<a id="tocScacheevictionsummary"></a>
<a id="tocscacheevictionsummary"></a>

```json
{
  "completed_at": "2019-08-24T14:15:22Z",
  "trigger": "string",
  "evicted_count": 0,
  "freed_bytes": 0,
  "blocked_count": 0,
  "reclaimable_bytes": 0,
  "usage_bytes_before": 0,
  "usage_bytes_after": 0,
  "free_bytes_before": 0,
  "free_bytes_after": 0
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|completed_at|string(date-time)|true|none|none|
|trigger|string|true|none|Trigger that initiated the latest eviction run.|
|evicted_count|integer(int32)|true|none|none|
|freed_bytes|integer(int64)|true|none|none|
|blocked_count|integer(int32)|true|none|none|
|reclaimable_bytes|integer(int64)|true|none|none|
|usage_bytes_before|integer(int64)|true|none|none|
|usage_bytes_after|integer(int64)|true|none|none|
|free_bytes_before|integer(int64)|true|none|none|
|free_bytes_after|integer(int64)|true|none|none|

<h2 id="tocS_CacheStatus">CacheStatus</h2>
<!-- backwards compatibility -->
<a id="schemacachestatus"></a>
<a id="schema_CacheStatus"></a>
<a id="tocScachestatus"></a>
<a id="tocscachestatus"></a>

```json
{
  "usage_bytes": 0,
  "configured_max_bytes": 0,
  "effective_max_bytes": 0,
  "reserve_bytes": 0,
  "high_watermark": 0,
  "low_watermark": 0,
  "min_state_age": "string",
  "store_total_bytes": 0,
  "store_free_bytes": 0,
  "state_count": 0,
  "reclaimable_bytes": 0,
  "blocked_count": 0,
  "pressure_reasons": [
    "string"
  ],
  "last_eviction": {
    "completed_at": "2019-08-24T14:15:22Z",
    "trigger": "string",
    "evicted_count": 0,
    "freed_bytes": 0,
    "blocked_count": 0,
    "reclaimable_bytes": 0,
    "usage_bytes_before": 0,
    "usage_bytes_after": 0,
    "free_bytes_before": 0,
    "free_bytes_after": 0
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|usage_bytes|integer(int64)|true|none|none|
|configured_max_bytes|any|false|none|Null when store-coupled mode is active.|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|integer(int64)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|effective_max_bytes|integer(int64)|true|none|none|
|reserve_bytes|integer(int64)|true|none|none|
|high_watermark|number|true|none|none|
|low_watermark|number|true|none|none|
|min_state_age|string|true|none|none|
|store_total_bytes|integer(int64)|true|none|none|
|store_free_bytes|integer(int64)|true|none|none|
|state_count|integer(int32)|true|none|none|
|reclaimable_bytes|integer(int64)|true|none|none|
|blocked_count|integer(int32)|true|none|none|
|pressure_reasons|[string]|true|none|none|
|last_eviction|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|[CacheEvictionSummary](#schemacacheevictionsummary)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

<h2 id="tocS_SourceContentHash">SourceContentHash</h2>
<!-- backwards compatibility -->
<a id="schemasourcecontenthash"></a>
<a id="schema_SourceContentHash"></a>
<a id="tocSsourcecontenthash"></a>
<a id="tocssourcecontenthash"></a>

```json
"string"

```

SHA-256 content hash with the `sha256:` prefix.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|SHA-256 content hash with the `sha256:` prefix.|

<h2 id="tocS_SourceManifestPath">SourceManifestPath</h2>
<!-- backwards compatibility -->
<a id="schemasourcemanifestpath"></a>
<a id="schema_SourceManifestPath"></a>
<a id="tocSsourcemanifestpath"></a>
<a id="tocssourcemanifestpath"></a>

```json
"string"

```

Workspace-relative slash-separated path. Remote requests must not send
absolute host paths, drive-qualified paths, or paths that escape the
workspace root.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|Workspace-relative slash-separated path. Remote requests must not send<br>absolute host paths, drive-qualified paths, or paths that escape the<br>workspace root.|

<h2 id="tocS_SourceWorkspaceRef">SourceWorkspaceRef</h2>
<!-- backwards compatibility -->
<a id="schemasourceworkspaceref"></a>
<a id="schema_SourceWorkspaceRef"></a>
<a id="tocSsourceworkspaceref"></a>
<a id="tocssourceworkspaceref"></a>

```json
{
  "root_id": "string",
  "root_path": "string",
  "work_dir": "string"
}

```

Client path context used to bind absolute arguments to the virtual workspace.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|root_id|string|false|none|Client-local workspace root identifier for diagnostics and retries.|
|root_path|string|true|none|Absolute logical path of the client workspace root, in the same OS<br>path flavor as file-bearing request arguments. It is a resolution<br>coordinate and is never opened as a server filesystem path.|
|work_dir|string|true|none|Absolute logical effective command working directory within<br>`root_path`, in the same OS path flavor. Plain `psql` includes are<br>resolved from this directory.|

<h2 id="tocS_SourceDirectoryEntry">SourceDirectoryEntry</h2>
<!-- backwards compatibility -->
<a id="schemasourcedirectoryentry"></a>
<a id="schema_SourceDirectoryEntry"></a>
<a id="tocSsourcedirectoryentry"></a>
<a id="tocssourcedirectoryentry"></a>

```json
{
  "name": "string",
  "kind": "file"
}

```

One entry in a complete workspace-relative directory listing.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|name|string|true|none|Directory-entry basename.|
|kind|string|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|kind|file|
|kind|directory|

<h2 id="tocS_SourceDirectoryListing">SourceDirectoryListing</h2>
<!-- backwards compatibility -->
<a id="schemasourcedirectorylisting"></a>
<a id="schema_SourceDirectoryListing"></a>
<a id="tocSsourcedirectorylisting"></a>
<a id="tocssourcedirectorylisting"></a>

```json
{
  "entries": [
    {
      "name": "string",
      "kind": "file"
    }
  ],
  "complete": true
}

```

Complete listing for the directory at the map key.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|entries|[[SourceDirectoryEntry](#schemasourcedirectoryentry)]|true|none|[One entry in a complete workspace-relative directory listing.]|
|complete|boolean|false|none|False values are ignored by the server and may cause another missing-input response.|

<h2 id="tocS_SourceManifest">SourceManifest</h2>
<!-- backwards compatibility -->
<a id="schemasourcemanifest"></a>
<a id="schema_SourceManifest"></a>
<a id="tocSsourcemanifest"></a>
<a id="tocssourcemanifest"></a>

```json
{
  "workspace_ref": {
    "root_id": "string",
    "root_path": "string",
    "work_dir": "string"
  },
  "files": {
    "property1": "string",
    "property2": "string"
  },
  "directories": {
    "property1": {
      "entries": [
        {
          "name": "string",
          "kind": "file"
        }
      ],
      "complete": true
    },
    "property2": {
      "entries": [
        {
          "name": "string",
          "kind": "file"
        }
      ],
      "complete": true
    }
  }
}

```

Optional remote-source manifest attached to file-bearing prepare and
cache-explain requests. The server uses this manifest with its scoped
content cache to build a virtual workspace for authoritative
format-specific input resolution. File-bearing request arguments retain
absolute client paths; `workspace_ref` binds those logical coordinates
to workspace-relative manifest keys. An empty manifest is a valid
first-round handshake and may produce `source_inputs_missing`.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|workspace_ref|[SourceWorkspaceRef](#schemasourceworkspaceref)|true|none|Client path context used to bind absolute arguments to the virtual workspace.|
|files|object|false|none|Map from workspace-relative source file path to verified client-side content hash.|
|» **additionalProperties**|[SourceContentHash](#schemasourcecontenthash)|false|none|SHA-256 content hash with the `sha256:` prefix.|
|directories|object|false|none|Map from workspace-relative directory path to complete directory listing.|
|» **additionalProperties**|[SourceDirectoryListing](#schemasourcedirectorylisting)|false|none|Complete listing for the directory at the map key.|

<h2 id="tocS_SourceMissingManifestEntry">SourceMissingManifestEntry</h2>
<!-- backwards compatibility -->
<a id="schemasourcemissingmanifestentry"></a>
<a id="schema_SourceMissingManifestEntry"></a>
<a id="tocSsourcemissingmanifestentry"></a>
<a id="tocssourcemissingmanifestentry"></a>

```json
{
  "path": "string",
  "kind": "file_hash",
  "reason": "string"
}

```

Manifest information the client should add before retrying the request.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|path|[SourceManifestPath](#schemasourcemanifestpath)|true|none|Workspace-relative slash-separated path. Remote requests must not send<br>absolute host paths, drive-qualified paths, or paths that escape the<br>workspace root.|
|kind|string|true|none|Type of manifest data the server needs to continue source resolution.|
|reason|string|false|none|Optional diagnostic reason such as `referenced_file` or `directory_traversal`.|

#### Enumerated Values

|Property|Value|
|---|---|
|kind|file_hash|
|kind|directory_listing|

<h2 id="tocS_SourceMissingBlob">SourceMissingBlob</h2>
<!-- backwards compatibility -->
<a id="schemasourcemissingblob"></a>
<a id="schema_SourceMissingBlob"></a>
<a id="tocSsourcemissingblob"></a>
<a id="tocssourcemissingblob"></a>

```json
{
  "path": "string",
  "hash": "string"
}

```

Source content blob absent from the server's scoped content cache.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|path|[SourceManifestPath](#schemasourcemanifestpath)|true|none|Workspace-relative slash-separated path. Remote requests must not send<br>absolute host paths, drive-qualified paths, or paths that escape the<br>workspace root.|
|hash|[SourceContentHash](#schemasourcecontenthash)|true|none|SHA-256 content hash with the `sha256:` prefix.|

<h2 id="tocS_SourceInputsMissingErrorResponse">SourceInputsMissingErrorResponse</h2>
<!-- backwards compatibility -->
<a id="schemasourceinputsmissingerrorresponse"></a>
<a id="schema_SourceInputsMissingErrorResponse"></a>
<a id="tocSsourceinputsmissingerrorresponse"></a>
<a id="tocssourceinputsmissingerrorresponse"></a>

```json
{
  "code": "source_inputs_missing",
  "message": "string",
  "missing_manifest_entries": [],
  "missing_blobs": []
}

```

Recoverable response used by remote clients. The client may upload the
missing blobs, expand the source manifest with requested hashes or
directory listings, and retry the original request.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|code|string|true|none|none|
|message|string|true|none|none|
|missing_manifest_entries|[[SourceMissingManifestEntry](#schemasourcemissingmanifestentry)]|false|none|[Manifest information the client should add before retrying the request.]|
|missing_blobs|[[SourceMissingBlob](#schemasourcemissingblob)]|false|none|[Source content blob absent from the server's scoped content cache.]|

#### Enumerated Values

|Property|Value|
|---|---|
|code|source_inputs_missing|

<h2 id="tocS_CacheExplainPrepareRequest">CacheExplainPrepareRequest</h2>
<!-- backwards compatibility -->
<a id="schemacacheexplainpreparerequest"></a>
<a id="schema_CacheExplainPrepareRequest"></a>
<a id="tocScacheexplainpreparerequest"></a>
<a id="tocscacheexplainpreparerequest"></a>

```json
{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  }
}

```

### Properties

oneOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[CacheExplainPrepareRequestPsql](#schemacacheexplainpreparerequestpsql)|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[CacheExplainPrepareRequestLiquibase](#schemacacheexplainpreparerequestliquibase)|false|none|none|

<h2 id="tocS_CacheExplainPrepareRequestPsql">CacheExplainPrepareRequestPsql</h2>
<!-- backwards compatibility -->
<a id="schemacacheexplainpreparerequestpsql"></a>
<a id="schema_CacheExplainPrepareRequestPsql"></a>
<a id="tocScacheexplainpreparerequestpsql"></a>
<a id="tocscacheexplainpreparerequestpsql"></a>

```json
{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|prepare_kind|string|true|none|Prepare adapter kind.|
|image_id|string|true|none|Base Docker image id to use.|
|psql_args|[string]|true|none|Arguments passed to `psql` (excluding connection flags). Argument<br>ordering is preserved. File paths must already be absolute and<br>normalized for the bound client execution context. With<br>`source_manifest`, they are logical client coordinates and are not<br>opened on the server filesystem.|
|stdin|string|false|none|SQL content to use for stdin when `psql_args` includes `-f -`.|
|source_manifest|[SourceManifest](#schemasourcemanifest)|false|none|Optional remote-source manifest attached to file-bearing prepare and<br>cache-explain requests. The server uses this manifest with its scoped<br>content cache to build a virtual workspace for authoritative<br>format-specific input resolution. File-bearing request arguments retain<br>absolute client paths; `workspace_ref` binds those logical coordinates<br>to workspace-relative manifest keys. An empty manifest is a valid<br>first-round handshake and may produce `source_inputs_missing`.|

#### Enumerated Values

|Property|Value|
|---|---|
|prepare_kind|psql|

<h2 id="tocS_CacheExplainPrepareRequestLiquibase">CacheExplainPrepareRequestLiquibase</h2>
<!-- backwards compatibility -->
<a id="schemacacheexplainpreparerequestliquibase"></a>
<a id="schema_CacheExplainPrepareRequestLiquibase"></a>
<a id="tocScacheexplainpreparerequestliquibase"></a>
<a id="tocscacheexplainpreparerequestliquibase"></a>

```json
{
  "prepare_kind": "lb",
  "image_id": "string",
  "liquibase_args": [
    "string"
  ],
  "liquibase_exec": "string",
  "liquibase_exec_mode": "string",
  "liquibase_env": {
    "property1": "string",
    "property2": "string"
  },
  "work_dir": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|prepare_kind|string|true|none|Prepare adapter kind.|
|image_id|string|true|none|Base Docker image id to use.|
|liquibase_args|[string]|true|none|Arguments passed to Liquibase. Argument ordering is preserved and<br>path-bearing arguments must already reflect the absolute bound<br>client execution context. With `source_manifest`, they are logical<br>client coordinates and are not opened on the server filesystem.|
|liquibase_exec|string|false|none|Optional Liquibase executable override selected by the CLI.|
|liquibase_exec_mode|string|false|none|Optional Liquibase executable mode selected by the CLI.|
|liquibase_env|object|false|none|Optional environment variables passed through to Liquibase.|
|» **additionalProperties**|string|false|none|none|
|work_dir|string|false|none|Absolute bound client working directory for the Liquibase invocation.|
|source_manifest|[SourceManifest](#schemasourcemanifest)|false|none|Optional remote-source manifest attached to file-bearing prepare and<br>cache-explain requests. The server uses this manifest with its scoped<br>content cache to build a virtual workspace for authoritative<br>format-specific input resolution. File-bearing request arguments retain<br>absolute client paths; `workspace_ref` binds those logical coordinates<br>to workspace-relative manifest keys. An empty manifest is a valid<br>first-round handshake and may produce `source_inputs_missing`.|

#### Enumerated Values

|Property|Value|
|---|---|
|prepare_kind|lb|

<h2 id="tocS_CacheExplainPrepareResponse">CacheExplainPrepareResponse</h2>
<!-- backwards compatibility -->
<a id="schemacacheexplainprepareresponse"></a>
<a id="schema_CacheExplainPrepareResponse"></a>
<a id="tocScacheexplainprepareresponse"></a>
<a id="tocscacheexplainprepareresponse"></a>

```json
{
  "decision": "hit",
  "reason_code": "string",
  "signature": "string",
  "matched_state_id": "string",
  "resolved_image_id": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|decision|string|true|none|Final-state cache decision for the bound prepare invocation.|
|reason_code|string|true|none|Diagnostic reason. The current implementation returns<br>`exact_state_match` for a hit and `no_matching_state` for a miss.<br>More specific miss classification is tracked in<br>https://github.com/kimak-irmagi/sqlrs/issues/92.|
|signature|string|true|none|Engine-computed final prepare signature used for cache lookup.|
|matched_state_id|any|false|none|Matching cached state id when `decision=hit`.|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|resolved_image_id|any|false|none|Resolved image id when the engine can report it.|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|decision|hit|
|decision|miss|

<h2 id="tocS_PrepareJobRequest">PrepareJobRequest</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobrequest"></a>
<a id="schema_PrepareJobRequest"></a>
<a id="tocSpreparejobrequest"></a>
<a id="tocspreparejobrequest"></a>

```json
{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  },
  "plan_only": true
}

```

### Properties

oneOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[PrepareJobRequestPsql](#schemapreparejobrequestpsql)|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[PrepareJobRequestLiquibase](#schemapreparejobrequestliquibase)|false|none|none|

<h2 id="tocS_PrepareJobRequestPsql">PrepareJobRequestPsql</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobrequestpsql"></a>
<a id="schema_PrepareJobRequestPsql"></a>
<a id="tocSpreparejobrequestpsql"></a>
<a id="tocspreparejobrequestpsql"></a>

```json
{
  "prepare_kind": "psql",
  "image_id": "string",
  "psql_args": [
    "string"
  ],
  "stdin": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  },
  "plan_only": true
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|prepare_kind|string|true|none|Prepare adapter kind.|
|image_id|string|true|none|Base Docker image id to use.|
|psql_args|[string]|true|none|Arguments passed to `psql` (excluding connection flags). Argument<br>ordering is preserved. File paths must be absolute. With<br>`source_manifest`, they are logical client coordinates and are not<br>opened on the server filesystem.|
|stdin|string|false|none|SQL content to use for stdin when `psql_args` includes `-f -`.|
|source_manifest|[SourceManifest](#schemasourcemanifest)|false|none|Optional remote-source manifest attached to file-bearing prepare and<br>cache-explain requests. The server uses this manifest with its scoped<br>content cache to build a virtual workspace for authoritative<br>format-specific input resolution. File-bearing request arguments retain<br>absolute client paths; `workspace_ref` binds those logical coordinates<br>to workspace-relative manifest keys. An empty manifest is a valid<br>first-round handshake and may produce `source_inputs_missing`.|
|plan_only|boolean|false|none|When true, only the plan is computed and no instance is created.|

#### Enumerated Values

|Property|Value|
|---|---|
|prepare_kind|psql|

<h2 id="tocS_PrepareJobRequestLiquibase">PrepareJobRequestLiquibase</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobrequestliquibase"></a>
<a id="schema_PrepareJobRequestLiquibase"></a>
<a id="tocSpreparejobrequestliquibase"></a>
<a id="tocspreparejobrequestliquibase"></a>

```json
{
  "prepare_kind": "lb",
  "image_id": "string",
  "liquibase_args": [
    "string"
  ],
  "liquibase_exec": "string",
  "liquibase_exec_mode": "string",
  "liquibase_env": {
    "property1": "string",
    "property2": "string"
  },
  "work_dir": "string",
  "source_manifest": {
    "workspace_ref": {
      "root_id": "string",
      "root_path": "string",
      "work_dir": "string"
    },
    "files": {
      "property1": "string",
      "property2": "string"
    },
    "directories": {
      "property1": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      },
      "property2": {
        "entries": [
          {
            "name": "string",
            "kind": "file"
          }
        ],
        "complete": true
      }
    }
  },
  "plan_only": true
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|prepare_kind|string|true|none|Prepare adapter kind.|
|image_id|string|true|none|Base Docker image id to use.|
|liquibase_args|[string]|true|none|Arguments passed to Liquibase. Argument ordering is preserved and<br>path-bearing arguments must already be normalized for the absolute<br>bound client execution context. With `source_manifest`, they are<br>logical client coordinates and are not opened on the server<br>filesystem.|
|liquibase_exec|string|false|none|Optional Liquibase executable override selected by the CLI.|
|liquibase_exec_mode|string|false|none|Optional Liquibase executable mode selected by the CLI.|
|liquibase_env|object|false|none|Optional environment variables passed through to Liquibase.|
|» **additionalProperties**|string|false|none|none|
|work_dir|string|false|none|Absolute bound client working directory for the Liquibase invocation.|
|source_manifest|[SourceManifest](#schemasourcemanifest)|false|none|Optional remote-source manifest attached to file-bearing prepare and<br>cache-explain requests. The server uses this manifest with its scoped<br>content cache to build a virtual workspace for authoritative<br>format-specific input resolution. File-bearing request arguments retain<br>absolute client paths; `workspace_ref` binds those logical coordinates<br>to workspace-relative manifest keys. An empty manifest is a valid<br>first-round handshake and may produce `source_inputs_missing`.|
|plan_only|boolean|false|none|When true, only the plan is computed and no instance is created.|

#### Enumerated Values

|Property|Value|
|---|---|
|prepare_kind|lb|

<h2 id="tocS_ConfigSetRequest">ConfigSetRequest</h2>
<!-- backwards compatibility -->
<a id="schemaconfigsetrequest"></a>
<a id="schema_ConfigSetRequest"></a>
<a id="tocSconfigsetrequest"></a>
<a id="tocsconfigsetrequest"></a>

```json
{
  "path": "string",
  "value": {}
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|path|string|true|none|JS-style path (e.g. `orchestrator.jobs.maxIdentical`).|
|value|[JSONValue](#schemajsonvalue)|true|none|none|

<h2 id="tocS_ConfigValue">ConfigValue</h2>
<!-- backwards compatibility -->
<a id="schemaconfigvalue"></a>
<a id="schema_ConfigValue"></a>
<a id="tocSconfigvalue"></a>
<a id="tocsconfigvalue"></a>

```json
{
  "path": "string",
  "value": {}
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|path|string|true|none|none|
|value|[JSONValue](#schemajsonvalue)|true|none|none|

<h2 id="tocS_ConfigMap">ConfigMap</h2>
<!-- backwards compatibility -->
<a id="schemaconfigmap"></a>
<a id="schema_ConfigMap"></a>
<a id="tocSconfigmap"></a>
<a id="tocsconfigmap"></a>

```json
{}

```

Full server config object.

### Properties

*None*

<h2 id="tocS_ConfigSchema">ConfigSchema</h2>
<!-- backwards compatibility -->
<a id="schemaconfigschema"></a>
<a id="schema_ConfigSchema"></a>
<a id="tocSconfigschema"></a>
<a id="tocsconfigschema"></a>

```json
{}

```

JSON Schema for config validation.

### Properties

*None*

<h2 id="tocS_JSONValue">JSONValue</h2>
<!-- backwards compatibility -->
<a id="schemajsonvalue"></a>
<a id="schema_JSONValue"></a>
<a id="tocSjsonvalue"></a>
<a id="tocsjsonvalue"></a>

```json
{}

```

### Properties

oneOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|null|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|number|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|integer|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|boolean|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|object|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[any]|false|none|none|

<h2 id="tocS_UserProfileWriteRequest">UserProfileWriteRequest</h2>
<!-- backwards compatibility -->
<a id="schemauserprofilewriterequest"></a>
<a id="schema_UserProfileWriteRequest"></a>
<a id="tocSuserprofilewriterequest"></a>
<a id="tocsuserprofilewriterequest"></a>

```json
{
  "display_name": "string",
  "email": "user@example.com"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|display_name|string|false|none|Optional profile display name.|
|email|string(email)|false|none|Optional profile email.|

<h2 id="tocS_UserProfileResult">UserProfileResult</h2>
<!-- backwards compatibility -->
<a id="schemauserprofileresult"></a>
<a id="schema_UserProfileResult"></a>
<a id="tocSuserprofileresult"></a>
<a id="tocsuserprofileresult"></a>

```json
{
  "status": "created",
  "user": {
    "id": "string",
    "display_name": "string",
    "email": "user@example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "identities": [
    {
      "provider": "oidc",
      "issuer": "string",
      "subject": "string"
    }
  ],
  "memberships": [
    {
      "organization": {
        "id": "string",
        "slug": "string",
        "display_name": "string",
        "created_at": "2019-08-24T14:15:22Z",
        "updated_at": "2019-08-24T14:15:22Z"
      },
      "membership": {
        "user_id": "string",
        "organization_id": "string",
        "role": "admin",
        "created_at": "2019-08-24T14:15:22Z"
      }
    }
  ]
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|status|string|false|none|Present when the response comes from a create or update command.|
|user|[UserProfile](#schemauserprofile)|true|none|none|
|identities|[[ExternalIdentity](#schemaexternalidentity)]|true|none|none|
|memberships|[[OrganizationMembershipView](#schemaorganizationmembershipview)]|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|status|created|
|status|existing|
|status|updated|

<h2 id="tocS_UserProfile">UserProfile</h2>
<!-- backwards compatibility -->
<a id="schemauserprofile"></a>
<a id="schema_UserProfile"></a>
<a id="tocSuserprofile"></a>
<a id="tocsuserprofile"></a>

```json
{
  "id": "string",
  "display_name": "string",
  "email": "user@example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "updated_at": "2019-08-24T14:15:22Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|true|none|Stable sqlrs user id.|
|display_name|string|true|none|none|
|email|any|true|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string(email)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|created_at|string(date-time)|true|none|none|
|updated_at|string(date-time)|true|none|none|

<h2 id="tocS_ExternalIdentity">ExternalIdentity</h2>
<!-- backwards compatibility -->
<a id="schemaexternalidentity"></a>
<a id="schema_ExternalIdentity"></a>
<a id="tocSexternalidentity"></a>
<a id="tocsexternalidentity"></a>

```json
{
  "provider": "oidc",
  "issuer": "string",
  "subject": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|provider|string|true|none|External identity provider kind.|
|issuer|string|true|none|OAuth/OIDC issuer identifier.|
|subject|string|true|none|Provider-local subject identifier.|

<h2 id="tocS_OrganizationCreateRequest">OrganizationCreateRequest</h2>
<!-- backwards compatibility -->
<a id="schemaorganizationcreaterequest"></a>
<a id="schema_OrganizationCreateRequest"></a>
<a id="tocSorganizationcreaterequest"></a>
<a id="tocsorganizationcreaterequest"></a>

```json
{
  "slug": "string",
  "display_name": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|slug|string|true|none|Lowercase organization slug, 3 to 63 characters.|
|display_name|string|false|none|Optional human-readable organization name.|

<h2 id="tocS_OrganizationCreateResponse">OrganizationCreateResponse</h2>
<!-- backwards compatibility -->
<a id="schemaorganizationcreateresponse"></a>
<a id="schema_OrganizationCreateResponse"></a>
<a id="tocSorganizationcreateresponse"></a>
<a id="tocsorganizationcreateresponse"></a>

```json
{
  "organization": {
    "id": "string",
    "slug": "string",
    "display_name": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "membership": {
    "user_id": "string",
    "organization_id": "string",
    "role": "admin",
    "created_at": "2019-08-24T14:15:22Z"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|organization|[Organization](#schemaorganization)|true|none|none|
|membership|[OrganizationMembership](#schemaorganizationmembership)|true|none|none|

<h2 id="tocS_OrganizationMembershipView">OrganizationMembershipView</h2>
<!-- backwards compatibility -->
<a id="schemaorganizationmembershipview"></a>
<a id="schema_OrganizationMembershipView"></a>
<a id="tocSorganizationmembershipview"></a>
<a id="tocsorganizationmembershipview"></a>

```json
{
  "organization": {
    "id": "string",
    "slug": "string",
    "display_name": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "updated_at": "2019-08-24T14:15:22Z"
  },
  "membership": {
    "user_id": "string",
    "organization_id": "string",
    "role": "admin",
    "created_at": "2019-08-24T14:15:22Z"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|organization|[Organization](#schemaorganization)|true|none|none|
|membership|[OrganizationMembership](#schemaorganizationmembership)|true|none|none|

<h2 id="tocS_Organization">Organization</h2>
<!-- backwards compatibility -->
<a id="schemaorganization"></a>
<a id="schema_Organization"></a>
<a id="tocSorganization"></a>
<a id="tocsorganization"></a>

```json
{
  "id": "string",
  "slug": "string",
  "display_name": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "updated_at": "2019-08-24T14:15:22Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|true|none|Stable sqlrs organization id.|
|slug|string|true|none|Stable human-readable organization reference.|
|display_name|string|true|none|none|
|created_at|string(date-time)|true|none|none|
|updated_at|string(date-time)|true|none|none|

<h2 id="tocS_OrganizationMembership">OrganizationMembership</h2>
<!-- backwards compatibility -->
<a id="schemaorganizationmembership"></a>
<a id="schema_OrganizationMembership"></a>
<a id="tocSorganizationmembership"></a>
<a id="tocsorganizationmembership"></a>

```json
{
  "user_id": "string",
  "organization_id": "string",
  "role": "admin",
  "created_at": "2019-08-24T14:15:22Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|user_id|string|true|none|none|
|organization_id|string|true|none|none|
|role|string|true|none|none|
|created_at|string(date-time)|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|role|admin|

<h2 id="tocS_PreparePlanTask">PreparePlanTask</h2>
<!-- backwards compatibility -->
<a id="schemaprepareplantask"></a>
<a id="schema_PreparePlanTask"></a>
<a id="tocSprepareplantask"></a>
<a id="tocsprepareplantask"></a>

```json
{
  "task_id": "string",
  "type": "plan",
  "planner_kind": "string"
}

```

### Properties

oneOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[PreparePlanTaskPlan](#schemaprepareplantaskplan)|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[PreparePlanTaskResolveImage](#schemaprepareplantaskresolveimage)|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[PreparePlanTaskStateExecute](#schemaprepareplantaskstateexecute)|false|none|none|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[PreparePlanTaskPrepareInstance](#schemaprepareplantaskprepareinstance)|false|none|none|

<h2 id="tocS_PreparePlanTaskInput">PreparePlanTaskInput</h2>
<!-- backwards compatibility -->
<a id="schemaprepareplantaskinput"></a>
<a id="schema_PreparePlanTaskInput"></a>
<a id="tocSprepareplantaskinput"></a>
<a id="tocsprepareplantaskinput"></a>

```json
{
  "kind": "image",
  "id": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|kind|string|true|none|none|
|id|string|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|kind|image|
|kind|state|

<h2 id="tocS_PreparePlanTaskPlan">PreparePlanTaskPlan</h2>
<!-- backwards compatibility -->
<a id="schemaprepareplantaskplan"></a>
<a id="schema_PreparePlanTaskPlan"></a>
<a id="tocSprepareplantaskplan"></a>
<a id="tocsprepareplantaskplan"></a>

```json
{
  "task_id": "string",
  "type": "plan",
  "planner_kind": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|task_id|string|true|none|none|
|type|string|true|none|none|
|planner_kind|string|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|type|plan|

<h2 id="tocS_PreparePlanTaskResolveImage">PreparePlanTaskResolveImage</h2>
<!-- backwards compatibility -->
<a id="schemaprepareplantaskresolveimage"></a>
<a id="schema_PreparePlanTaskResolveImage"></a>
<a id="tocSprepareplantaskresolveimage"></a>
<a id="tocsprepareplantaskresolveimage"></a>

```json
{
  "task_id": "string",
  "type": "resolve_image",
  "image_id": "string",
  "resolved_image_id": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|task_id|string|true|none|none|
|type|string|true|none|none|
|image_id|string|true|none|none|
|resolved_image_id|string|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|type|resolve_image|

<h2 id="tocS_PreparePlanTaskStateExecute">PreparePlanTaskStateExecute</h2>
<!-- backwards compatibility -->
<a id="schemaprepareplantaskstateexecute"></a>
<a id="schema_PreparePlanTaskStateExecute"></a>
<a id="tocSprepareplantaskstateexecute"></a>
<a id="tocsprepareplantaskstateexecute"></a>

```json
{
  "task_id": "string",
  "type": "state_execute",
  "input": {
    "kind": "image",
    "id": "string"
  },
  "task_hash": "string",
  "output_state_id": "string",
  "cached": true
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|task_id|string|true|none|none|
|type|string|true|none|none|
|input|[PreparePlanTaskInput](#schemaprepareplantaskinput)|true|none|none|
|task_hash|string|true|none|none|
|output_state_id|string|true|none|none|
|cached|boolean|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|type|state_execute|

<h2 id="tocS_PreparePlanTaskPrepareInstance">PreparePlanTaskPrepareInstance</h2>
<!-- backwards compatibility -->
<a id="schemaprepareplantaskprepareinstance"></a>
<a id="schema_PreparePlanTaskPrepareInstance"></a>
<a id="tocSprepareplantaskprepareinstance"></a>
<a id="tocsprepareplantaskprepareinstance"></a>

```json
{
  "task_id": "string",
  "type": "prepare_instance",
  "input": {
    "kind": "image",
    "id": "string"
  },
  "instance_mode": "ephemeral"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|task_id|string|true|none|none|
|type|string|true|none|none|
|input|[PreparePlanTaskInput](#schemaprepareplantaskinput)|true|none|none|
|instance_mode|string|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|type|prepare_instance|
|instance_mode|ephemeral|

<h2 id="tocS_PrepareJobAccepted">PrepareJobAccepted</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobaccepted"></a>
<a id="schema_PrepareJobAccepted"></a>
<a id="tocSpreparejobaccepted"></a>
<a id="tocspreparejobaccepted"></a>

```json
{
  "job_id": "string",
  "status_url": "string",
  "events_url": "string",
  "status": "queued"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|job_id|string|true|none|Prepare job id.|
|status_url|string|true|none|URL for job status polling.|
|events_url|string|false|none|URL for job event streaming.|
|status|string|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|status|queued|

<h2 id="tocS_PrepareJobStatus">PrepareJobStatus</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobstatus"></a>
<a id="schema_PrepareJobStatus"></a>
<a id="tocSpreparejobstatus"></a>
<a id="tocspreparejobstatus"></a>

```json
{
  "job_id": "string",
  "status": "queued",
  "prepare_kind": "string",
  "image_id": "string",
  "plan_only": true,
  "prepare_args_normalized": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "started_at": "2019-08-24T14:15:22Z",
  "finished_at": "2019-08-24T14:15:22Z",
  "tasks": [
    {
      "task_id": "string",
      "type": "plan",
      "planner_kind": "string"
    }
  ],
  "result": {
    "dsn": "string",
    "instance_id": "string",
    "state_id": "string",
    "image_id": "string",
    "prepare_kind": "string",
    "prepare_args_normalized": "string"
  },
  "error": {
    "code": "string",
    "message": "string",
    "details": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|job_id|string|true|none|none|
|status|string|true|none|none|
|prepare_kind|string|true|none|none|
|image_id|string|true|none|none|
|plan_only|boolean|false|none|True when the job computes a plan only.|
|prepare_args_normalized|string|false|none|Normalized prepare arguments (when available).|
|created_at|string(date-time)|false|none|none|
|started_at|string(date-time)|false|none|none|
|finished_at|string(date-time)|false|none|none|
|tasks|[[PreparePlanTask](#schemaprepareplantask)]|false|none|Present when the plan is available (plan-only or planned job).|
|result|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|[PrepareJobResult](#schemapreparejobresult)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|error|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|[ErrorResponse](#schemaerrorresponse)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|status|queued|
|status|running|
|status|succeeded|
|status|failed|

<h2 id="tocS_PrepareJobEntry">PrepareJobEntry</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobentry"></a>
<a id="schema_PrepareJobEntry"></a>
<a id="tocSpreparejobentry"></a>
<a id="tocspreparejobentry"></a>

```json
{
  "job_id": "string",
  "status": "queued",
  "prepare_kind": "string",
  "image_id": "string",
  "resolved_image_id": "string",
  "prepare_args_normalized": "string",
  "signature": "string",
  "plan_only": true,
  "created_at": "2019-08-24T14:15:22Z",
  "started_at": "2019-08-24T14:15:22Z",
  "finished_at": "2019-08-24T14:15:22Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|job_id|string|true|none|none|
|status|string|true|none|none|
|prepare_kind|string|true|none|none|
|image_id|string|true|none|Original requested image reference for the job.|
|resolved_image_id|string|false|none|Resolved digest-based image reference when available.|
|prepare_args_normalized|string|false|none|Normalized prepare arguments when available for the job entry.|
|signature|string|false|none|Engine-computed job signature used for retention and drift diagnostics when available.|
|plan_only|boolean|false|none|True when the job computes a plan only.|
|created_at|string(date-time)|false|none|none|
|started_at|string(date-time)|false|none|none|
|finished_at|string(date-time)|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|status|queued|
|status|running|
|status|succeeded|
|status|failed|

<h2 id="tocS_PrepareJobEvent">PrepareJobEvent</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobevent"></a>
<a id="schema_PrepareJobEvent"></a>
<a id="tocSpreparejobevent"></a>
<a id="tocspreparejobevent"></a>

```json
{
  "type": "status",
  "ts": "2019-08-24T14:15:22Z",
  "task_id": "string",
  "status": "queued",
  "message": "string",
  "result": {
    "dsn": "string",
    "instance_id": "string",
    "state_id": "string",
    "image_id": "string",
    "prepare_kind": "string",
    "prepare_args_normalized": "string"
  },
  "error": {
    "code": "string",
    "message": "string",
    "details": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|type|string|true|none|none|
|ts|string(date-time)|true|none|none|
|task_id|string|false|none|Present for task status events.|
|status|string|false|none|none|
|message|string|false|none|none|
|result|[PrepareJobResult](#schemapreparejobresult)|false|none|none|
|error|[ErrorResponse](#schemaerrorresponse)|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|type|status|
|type|log|
|type|result|
|type|error|
|type|task|
|status|queued|
|status|running|
|status|succeeded|
|status|failed|

<h2 id="tocS_PrepareJobResult">PrepareJobResult</h2>
<!-- backwards compatibility -->
<a id="schemapreparejobresult"></a>
<a id="schema_PrepareJobResult"></a>
<a id="tocSpreparejobresult"></a>
<a id="tocspreparejobresult"></a>

```json
{
  "dsn": "string",
  "instance_id": "string",
  "state_id": "string",
  "image_id": "string",
  "prepare_kind": "string",
  "prepare_args_normalized": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|dsn|string|true|none|DSN for the prepared instance.|
|instance_id|string|true|none|none|
|state_id|string|true|none|none|
|image_id|string|true|none|none|
|prepare_kind|string|true|none|none|
|prepare_args_normalized|string|true|none|none|

<h2 id="tocS_RunRequest">RunRequest</h2>
<!-- backwards compatibility -->
<a id="schemarunrequest"></a>
<a id="schema_RunRequest"></a>
<a id="tocSrunrequest"></a>
<a id="tocsrunrequest"></a>

```json
{
  "instance_ref": "string",
  "kind": "psql",
  "command": "string",
  "args": [
    "string"
  ],
  "stdin": "string",
  "steps": [
    {
      "args": [
        "string"
      ],
      "stdin": "string"
    }
  ]
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|instance_ref|string|true|none|Instance id or name.|
|kind|string|true|none|none|
|command|string,null|false|none|Optional command; defaults to the run kind command.|
|args|[string]|true|none|Command arguments; applies to the default command when `command` is omitted. Ignored when `steps` is provided.|
|stdin|string,null|false|none|Optional stdin payload for the run command.|
|steps|[[RunStep](#schemarunstep)]|false|none|Optional psql run steps (mutually exclusive with args/stdin).|

#### Enumerated Values

|Property|Value|
|---|---|
|kind|psql|
|kind|pgbench|

<h2 id="tocS_RunStep">RunStep</h2>
<!-- backwards compatibility -->
<a id="schemarunstep"></a>
<a id="schema_RunStep"></a>
<a id="tocSrunstep"></a>
<a id="tocsrunstep"></a>

```json
{
  "args": [
    "string"
  ],
  "stdin": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|args|[string]|true|none|none|
|stdin|string,null|false|none|Optional stdin payload for this step.|

<h2 id="tocS_RunEvent">RunEvent</h2>
<!-- backwards compatibility -->
<a id="schemarunevent"></a>
<a id="schema_RunEvent"></a>
<a id="tocSrunevent"></a>
<a id="tocsrunevent"></a>

```json
{
  "type": "start",
  "ts": "2019-08-24T14:15:22Z",
  "instance_id": "string",
  "data": "string",
  "exit_code": 0,
  "error": {
    "code": "string",
    "message": "string",
    "details": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|type|string|true|none|none|
|ts|string(date-time)|true|none|none|
|instance_id|string|false|none|Present for start events.|
|data|string|false|none|Output chunk for stdout/stderr events or log message text.|
|exit_code|integer(int32)|false|none|Process exit code for exit events.|
|error|[ErrorResponse](#schemaerrorresponse)|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|type|start|
|type|stdout|
|type|stderr|
|type|exit|
|type|error|
|type|log|

<h2 id="tocS_TaskEntry">TaskEntry</h2>
<!-- backwards compatibility -->
<a id="schemataskentry"></a>
<a id="schema_TaskEntry"></a>
<a id="tocStaskentry"></a>
<a id="tocstaskentry"></a>

```json
{
  "task_id": "string",
  "job_id": "string",
  "type": "plan",
  "status": "queued",
  "planner_kind": "string",
  "input": {
    "kind": "image",
    "id": "string"
  },
  "task_hash": "string",
  "output_state_id": "string",
  "cached": true,
  "instance_mode": "ephemeral",
  "image_id": "string",
  "resolved_image_id": "string",
  "args_summary": "string",
  "changeset_id": "string",
  "changeset_author": "string",
  "changeset_path": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|task_id|string|true|none|none|
|job_id|string|true|none|none|
|type|string|true|none|none|
|status|string|true|none|none|
|planner_kind|string|false|none|none|
|input|[PreparePlanTaskInput](#schemaprepareplantaskinput)|false|none|none|
|task_hash|string|false|none|none|
|output_state_id|string|false|none|none|
|cached|boolean|false|none|none|
|instance_mode|string|false|none|none|
|image_id|string|false|none|Requested image reference relevant to the task, when applicable.|
|resolved_image_id|string|false|none|Resolved digest-based image reference relevant to the task, when available.|
|args_summary|string|false|none|Stable one-line summary for human-oriented task listings.|
|changeset_id|string|false|none|none|
|changeset_author|string|false|none|none|
|changeset_path|string|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|type|plan|
|type|resolve_image|
|type|state_execute|
|type|prepare_instance|
|status|queued|
|status|running|
|status|succeeded|
|status|failed|
|instance_mode|ephemeral|

<h2 id="tocS_DeleteResult">DeleteResult</h2>
<!-- backwards compatibility -->
<a id="schemadeleteresult"></a>
<a id="schema_DeleteResult"></a>
<a id="tocSdeleteresult"></a>
<a id="tocsdeleteresult"></a>

```json
{
  "dry_run": true,
  "outcome": "deleted",
  "root": {
    "kind": "state",
    "id": "string",
    "connections": 0,
    "runtime_id": "string",
    "blocked": "active_connections",
    "children": [
      {}
    ]
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|dry_run|boolean|true|none|none|
|outcome|string|true|none|none|
|root|[DeleteTreeNode](#schemadeletetreenode)|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|outcome|deleted|
|outcome|would_delete|
|outcome|blocked|

<h2 id="tocS_DeleteTreeNode">DeleteTreeNode</h2>
<!-- backwards compatibility -->
<a id="schemadeletetreenode"></a>
<a id="schema_DeleteTreeNode"></a>
<a id="tocSdeletetreenode"></a>
<a id="tocsdeletetreenode"></a>

```json
{
  "kind": "state",
  "id": "string",
  "connections": 0,
  "runtime_id": "string",
  "blocked": "active_connections",
  "children": [
    {
      "kind": "state",
      "id": "string",
      "connections": 0,
      "runtime_id": "string",
      "blocked": "active_connections",
      "children": []
    }
  ]
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|kind|string|true|none|none|
|id|string|true|none|none|
|connections|integer(int32)|false|none|Active connections (instances only).|
|runtime_id|string|false|none|Runtime/container identifier for instance nodes.|
|blocked|string|false|none|Reason the node cannot be deleted.|
|children|[[DeleteTreeNode](#schemadeletetreenode)]|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|kind|state|
|kind|instance|
|kind|job|
|blocked|active_connections|
|blocked|active_tasks|
|blocked|has_descendants|
|blocked|blocked_by_descendant|

<h2 id="tocS_ErrorResponse">ErrorResponse</h2>
<!-- backwards compatibility -->
<a id="schemaerrorresponse"></a>
<a id="schema_ErrorResponse"></a>
<a id="tocSerrorresponse"></a>
<a id="tocserrorresponse"></a>

```json
{
  "code": "string",
  "message": "string",
  "details": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|code|string|true|none|none|
|message|string|true|none|none|
|details|string|false|none|none|

<h2 id="tocS_NameEntry">NameEntry</h2>
<!-- backwards compatibility -->
<a id="schemanameentry"></a>
<a id="schema_NameEntry"></a>
<a id="tocSnameentry"></a>
<a id="tocsnameentry"></a>

```json
{
  "name": "string",
  "instance_id": "string",
  "image_id": "string",
  "state_id": "string",
  "state_fingerprint": "string",
  "status": "active",
  "last_used_at": "2019-08-24T14:15:22Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|name|string|true|none|none|
|instance_id|string,null|false|none|none|
|image_id|string|true|none|none|
|state_id|string|true|none|none|
|state_fingerprint|string|false|none|none|
|status|string|true|none|none|
|last_used_at|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string(date-time)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|status|active|
|status|missing|
|status|expired|

<h2 id="tocS_InstanceEntry">InstanceEntry</h2>
<!-- backwards compatibility -->
<a id="schemainstanceentry"></a>
<a id="schema_InstanceEntry"></a>
<a id="tocSinstanceentry"></a>
<a id="tocsinstanceentry"></a>

```json
{
  "instance_id": "string",
  "image_id": "string",
  "state_id": "string",
  "name": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "expires_at": "2019-08-24T14:15:22Z",
  "status": "active"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|instance_id|string|true|none|none|
|image_id|string|true|none|none|
|state_id|string|true|none|none|
|name|string,null|false|none|none|
|created_at|string(date-time)|true|none|none|
|expires_at|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string(date-time)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|status|string|true|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|status|active|
|status|expired|
|status|orphaned|

<h2 id="tocS_StateEntry">StateEntry</h2>
<!-- backwards compatibility -->
<a id="schemastateentry"></a>
<a id="schema_StateEntry"></a>
<a id="tocSstateentry"></a>
<a id="tocsstateentry"></a>

```json
{
  "state_id": "string",
  "parent_state_id": "string",
  "image_id": "string",
  "prepare_kind": "string",
  "prepare_args_normalized": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "size_bytes": 0,
  "last_used_at": "2019-08-24T14:15:22Z",
  "use_count": 0,
  "min_retention_until": "2019-08-24T14:15:22Z",
  "refcount": 0
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|state_id|string|true|none|none|
|parent_state_id|any|false|none|Parent state id (null for roots).|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|image_id|string|true|none|none|
|prepare_kind|string|true|none|none|
|prepare_args_normalized|string|true|none|none|
|created_at|string(date-time)|true|none|none|
|size_bytes|integer(int64)|false|none|none|
|last_used_at|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string(date-time)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|use_count|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|integer(int64)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|min_retention_until|any|false|none|none|

anyOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|string(date-time)|false|none|none|

or

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|» *anonymous*|null|false|none|none|

continued

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|refcount|integer(int32)|true|none|none|

