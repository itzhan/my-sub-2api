# GPT Image API 对接文档

sub2api 兼容 OpenAI 官方 Images API 的协议，支持文生图与图片编辑两种能力。接入方式与调用 OpenAI `/v1/images/generations` 完全一致，只需把 `base_url` 替换成你的 sub2api 网关地址即可。

---

## 1. 前置条件

- **API Key 必须绑定在 "OpenAI" 平台的 group 上**。如果 key 绑的是 Anthropic / Gemini 等 group，调用图像接口会返回 404：
  ```json
  {"error":{"type":"not_found_error","message":"Images API is not supported for this platform"}}
  ```
- 网关地址示例：`https://your-gateway.example.com`（下文统一用 `{BASE}` 指代）

---

## 2. 支持的模型

| Model ID       | 说明              |
|----------------|-----------------|
| `gpt-image-1`  | 第一代 GPT Image 模型 |
| `gpt-image-1.5`| 1.5 代模型         |
| `gpt-image-2`  | **默认**，第二代模型    |

> 不传 `model` 字段时，服务端会默认使用 `gpt-image-2`。

---

## 3. 端点

| Method | Path                    | 作用          |
|--------|-------------------------|-------------|
| POST   | `/v1/images/generations`| 文生图         |
| POST   | `/v1/images/edits`      | 图片编辑（含 mask）|

> 不带 `/v1` 前缀的 `/images/generations` 与 `/images/edits` 同样可用。

---

## 4. 认证

通过 HTTP 头 `Authorization: Bearer <API_KEY>`。

```
Authorization: Bearer sk-xxxxxxxxxxxxxxxx
```

---

## 5. `/v1/images/generations` — 文生图

**Content-Type**: `application/json`

### 请求字段

| 字段                | 类型        | 必填 | 说明                                                                                              |
|---------------------|-------------|-----|-------------------------------------------------------------------------------------------------|
| `model`             | string      | 否   | 模型 ID，见 §2，省略时为 `gpt-image-2`                                                                   |
| `prompt`            | string      | 是   | 文本提示词                                                                                           |
| `n`                 | integer     | 否   | 生成张数，正整数，默认 `1`                                                                                 |
| `size`              | string      | 否   | 图像尺寸，见下方支持列表，默认 `auto`                                                                         |
| `response_format`   | string      | 否   | `url`（默认）或 `b64_json`                                                                            |
| `stream`            | boolean     | 否   | 是否使用 SSE 流式返回，默认 `false`                                                                        |
| `background`        | string      | 否   | 原生参数：背景透明度等                                                                                     |
| `quality`           | string      | 否   | 原生参数：图像质量等级                                                                                     |
| `style`             | string      | 否   | 原生参数：风格                                                                                         |
| `output_format`     | string      | 否   | 原生参数：输出格式                                                                                       |
| `output_compression`| integer     | 否   | 原生参数：输出压缩率                                                                                      |
| `moderation`        | string      | 否   | 原生参数：审核策略                                                                                       |

### 支持的 `size` 取值

- `1024x1024`（1K）
- `1536x1024`（2K 横版）
- `1024x1536`（2K 竖版）
- `1792x1024`（2K 超宽横版）
- `1024x1792`（2K 超高竖版）
- `auto` / 不传（默认 2K 策略）

其它尺寸会被服务端按 2K 档位处理。

### 示例请求

```bash
curl -X POST {BASE}/v1/images/generations \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "a cat sitting on a wooden table, photorealistic, soft morning light",
    "n": 1,
    "size": "1024x1024"
  }'
```

### 响应示例（与 OpenAI 官方完全兼容）

```json
{
  "created": 1735689600,
  "data": [
    {
      "url": "https://...generated-image-url..."
    }
  ]
}
```

当 `response_format=b64_json` 时：

```json
{
  "created": 1735689600,
  "data": [
    {
      "b64_json": "iVBORw0KGgoAAAANSU..."
    }
  ]
}
```

### 流式响应（stream=true）

返回 SSE 事件流，事件格式与 OpenAI 官方一致。客户端按 `text/event-stream` 解析即可。

---

## 6. `/v1/images/edits` — 图片编辑

**Content-Type**: `multipart/form-data`（必须上传文件，不支持纯 JSON）

### 字段

| 字段              | 类型               | 必填 | 说明                          |
|-------------------|--------------------|-----|-----------------------------|
| `image`           | file（PNG/JPEG/WEBP）| 是   | 待编辑原图                       |
| `mask`            | file               | 否   | 可选遮罩图（透明像素区域为可编辑）           |
| `prompt`          | string             | 是   | 编辑指令                         |
| `model`           | string             | 否   | 默认 `gpt-image-2`             |
| `n`               | integer            | 否   | 张数                           |
| `size`            | string             | 否   | 同文生图                         |
| `response_format` | string             | 否   | `url` 或 `b64_json`           |
| 其它原生字段       | —                  | 否   | `background` / `quality` 等同上 |

### 示例请求

```bash
curl -X POST {BASE}/v1/images/edits \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx" \
  -F "model=gpt-image-2" \
  -F "prompt=make the sky purple and add stars" \
  -F "image=@./input.png" \
  -F "mask=@./mask.png"
```

响应结构同 `/generations`。

---

## 7. 错误码

| HTTP | 场景                                               |
|------|--------------------------------------------------|
| 400  | 参数错误（如 `prompt` 为空、`n` 非正整数、edits 缺 `image` 文件）    |
| 401  | API Key 无效或过期                                     |
| 403  | 权限不足 / Key 被禁用                                    |
| 404  | Key 对应 group 不是 OpenAI 平台（Images API 不支持非 OpenAI） |
| 429  | 上游账号 rate limit 或配额耗尽                             |
| 5xx  | 上游或网关错误                                          |

错误响应体：
```json
{
  "error": {
    "type": "...",
    "message": "...",
    "code": "..."
  }
}
```

---

## 8. SDK 接入示例

### OpenAI Python SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="{BASE}/v1",
    api_key="sk-xxxxxxxxxxxxxxxx",
)

resp = client.images.generate(
    model="gpt-image-2",
    prompt="a cat sitting on a wooden table",
    n=1,
    size="1024x1024",
)
print(resp.data[0].url)
```

### OpenAI Node SDK

```javascript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "{BASE}/v1",
  apiKey: "sk-xxxxxxxxxxxxxxxx",
});

const res = await client.images.generate({
  model: "gpt-image-2",
  prompt: "a cat sitting on a wooden table",
  n: 1,
  size: "1024x1024",
});
console.log(res.data[0].url);
```

---

## 9. 计费

图像生成按 **尺寸档位 + 模型** 计价：
- `1024x1024` → 1K 档
- 其它合法尺寸 → 2K 档

具体单价见后台「账号统计价格」或「渠道定价」配置。

---

## 10. 常见排障

| 症状                                 | 可能原因                                    |
|------------------------------------|-----------------------------------------|
| 404 "Images API is not supported"  | Key 绑错 group 平台，需换为 OpenAI group        |
| 400 "image file is required"       | 调了 `/edits` 但没上传 `image` 文件              |
| 400 "n must be a positive integer" | `n` 字段传了 0 或负数                          |
| 429                                | 上游账号额度 / rate limit 触发，稍后重试或后台换账号       |
| 上传文件被拒                             | 超过网关 body size，参考 master `.env` 里 body-limit |
