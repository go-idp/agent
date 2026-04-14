# Agent 文件分片上传接口（`/files/append`）

本文档说明 agent 侧用于大文件流式上传的接口能力。

## 接口

- **Method**: `POST`
- **Path**: `/files/append`
- **Auth**: Basic Auth（与 agent 现有接口一致）
- **Content-Type**: `application/octet-stream`

## Query 参数

- `path`（必填）：目标文件绝对路径，如 `/tmp/upload.bin`
- `truncate`（可选）：`true|false`
  - `true`: 先清空/新建目标文件，再写入当前分片（通常用于第 1 片）
  - `false`: 追加写入（用于后续分片）

## Body

- 原始二进制分片内容（不是 base64 文本）

## 返回

成功返回 `ctx.Success(...)` 包含：

- `path`: 目标路径
- `size`: 本次写入字节数
- `mode`: 固定为 `append`
- `reset`: 是否执行了 truncate

失败返回 `ctx.Fail(...)`，常见错误：

- `path is required`
- `path must be absolute`
- `path must be file path`
- `parent directory not found`
- `failed to open file`
- `failed to write file`

## 推荐上传流程

1. 客户端按固定分片大小切分文件（如 256KB）
2. 第 1 片调用：`truncate=true`
3. 后续片调用：`truncate=false`
4. 全部分片成功后，调用业务侧回读/校验接口确认落地

## curl 示例

```bash
# 第 1 片（truncate）
curl -u "<client_id>:<client_secret>" \
  -X POST \
  --data-binary @chunk_0.bin \
  "http://127.0.0.1:8838/files/append?path=/tmp/demo.bin&truncate=true"

# 第 2 片（append）
curl -u "<client_id>:<client_secret>" \
  -X POST \
  --data-binary @chunk_1.bin \
  "http://127.0.0.1:8838/files/append?path=/tmp/demo.bin&truncate=false"
```

