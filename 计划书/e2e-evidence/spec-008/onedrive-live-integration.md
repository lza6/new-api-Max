# Spec-008 OneDrive 真实集成测试证据

**日期**：2026-10-14 | **代码**：`a76da96b5` | **环境**：生产机 Go 容器（本机无 Docker）

## 命令

```bash
docker run --rm -v /root/conf-src:/src -w /src \
 -e TRAINING_ONEDRIVE_TENANT="ai654outlook.onmicrosoft.com" \
 -e TRAINING_ONEDRIVE_CLIENT_ID="816cb6d8-..." \
 -e TRAINING_ONEDRIVE_CLIENT_SECRET="Vz_8Q~..." \
 -e TRAINING_ONEDRIVE_DRIVE_ID="b!fhXgR7uN..." \
 golang:1.25 sh -c "go test ./service/training_capture/ -run TestOneDriveLive -count=1 -v"
```

## 结果：**9/9 PASS**（真实云盘，非 mock）

```
--- PASS: TestOneDriveLive (5.15s)
    --- PASS: EnsureDirIsIdempotent          # 重复建目录不报错（幂等）
    --- PASS: UploadAndReadBackExactBytes    # 上传成功并返回远端引用
    --- PASS: ListReturnsUploadedPart        # 列举能看到刚上传的分片
    --- PASS: PickNextPartSeqFromRealListing # 真实列举恢复分片序号 → 2
    --- PASS: StatReturnsRealQuota           # 真实配额读回
    --- PASS: ListMissingDirectoryReturnsEmptyNotError  # 目录不存在→空，不报错
    --- PASS: WritePartRejectsOversize       # >4MB 明确报错（不截断）
    --- PASS: WritePartRejectsEmpty          # 空 payload 报错
    --- PASS: CountItemsCountsFoldersAndFiles
```

实测输出：
- **云盘配额：总 5.00TB / 已用 0.14MB / 剩余 5.00TB**
- 根目录统计：2 个目录 / 1 个文件

## 本轮覆盖的能力

| 能力 | 状态 |
|---|---|
| client_credentials 取 token | ✅ 真实 |
| 幂等建目录（`conflictBehavior=replace`） | ✅ 真实 |
| 上传分片（PUT `.../content`） | ✅ 真实 |
| 列举（恢复分片序号） | ✅ 真实 |
| 配额读取 | ✅ 真实 |
| 超限/空 payload 拒绝 | ✅ 真实 |

## 实测踩坑（已写进实现）

1. **读回必须跟随重定向**：`.../content` 返回 302 到 SharePoint 下载地址，
   不跟随会拿到空响应体（表现像「上传成功但读不到」）。
2. **建目录必须 `conflictBehavior=replace`**：用默认值会 409；
   用 `rename` 会产生 `user-3 1` 这类垃圾目录。
3. 简单上传上限 4MB（Graph 限制），故分片阈值必须设在 4MB 以下。
