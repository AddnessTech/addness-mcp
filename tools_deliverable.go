package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func listDeliverablesTool() mcp.Tool {
	return mcp.NewTool("list_deliverables",
		mcp.WithDescription("Goalに紐づく成果物（リソース）一覧を取得する。ファイル・ドキュメント・リンク・フォルダの階層構造で管理される。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleListDeliverables(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v1/team/objectives/%s/deliverables?limit=100", goalID))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		deliverables, err := parseDeliverableList(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(deliverables) == 0 {
			return textResult("No deliverables found."), nil
		}

		return textResult(formatDeliverableList(deliverables)), nil
	}
}

func getDeliverableTool() mcp.Tool {
	return mcp.NewTool("get_deliverable",
		mcp.WithDescription("成果物の詳細を取得する。ドキュメントの本文・ファイルのダウンロードURLなどを含む。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("deliverable_id",
			mcp.Required(),
			mcp.Description("Deliverable ID (short ID)"),
		),
	)
}

func handleGetDeliverable(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		deliverableID, err := client.ids.Resolve(argStr(args, "deliverable_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v1/team/objectives/%s/deliverables/%s", goalID, deliverableID))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		d, err := parseDeliverable(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		return textResult(formatDeliverableDetail(d)), nil
	}
}

func createDeliverableTool() mcp.Tool {
	return mcp.NewTool("create_deliverable",
		mcp.WithDescription(
			"Goalに成果物を追加する。ドキュメント（テキスト）・リンク・フォルダを作成可能。"+
				"ファイルアップロードの場合はnode_type='file'とfile_name・file_pathを指定すると自動アップロードされる。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("node_type",
			mcp.Required(),
			mcp.Description("Type of deliverable"),
			mcp.Enum("document", "file", "link", "folder"),
		),
		mcp.WithString("display_name",
			mcp.Required(),
			mcp.Description("Display name for the deliverable"),
		),
		mcp.WithString("content",
			mcp.Description("Content text (for document type)"),
		),
		mcp.WithString("link_url",
			mcp.Description("URL (for link type)"),
		),
		mcp.WithString("file_name",
			mcp.Description("File name with extension (for file type, e.g. 'report.pdf')"),
		),
		mcp.WithString("file_path",
			mcp.Description("Local file path to upload (for file type). The file will be uploaded automatically."),
		),
		mcp.WithString("parent_id",
			mcp.Description("Parent deliverable ID (short ID) to create inside a folder"),
		),
	)
}

func handleCreateDeliverable(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		body := map[string]any{
			"nodeType":    argStr(args, "node_type"),
			"displayName": argStr(args, "display_name"),
		}
		if v := argStr(args, "content"); v != "" {
			body["content"] = v
		}
		if v := argStr(args, "link_url"); v != "" {
			body["linkUrl"] = v
		}
		if v := argStr(args, "file_name"); v != "" {
			fileInfo := map[string]any{
				"fileName": v,
			}
			// Detect content type from extension
			switch strings.ToLower(filepath.Ext(v)) {
			case ".png":
				fileInfo["contentType"] = "image/png"
			case ".jpg", ".jpeg":
				fileInfo["contentType"] = "image/jpeg"
			case ".gif":
				fileInfo["contentType"] = "image/gif"
			case ".pdf":
				fileInfo["contentType"] = "application/pdf"
			case ".txt":
				fileInfo["contentType"] = "text/plain"
			default:
				fileInfo["contentType"] = "application/octet-stream"
			}
			// Get file size if file_path is provided
			if fp := argStr(args, "file_path"); fp != "" {
				if stat, err := os.Stat(fp); err == nil {
					fileInfo["fileSize"] = stat.Size()
				}
			}
			body["file"] = fileInfo
		}
		if v := argStr(args, "parent_id"); v != "" {
			resolved, err := client.ids.Resolve(v)
			if err != nil {
				return errResult(err.Error()), nil
			}
			body["parentDeliverableId"] = resolved
		}

		data, err := client.Post(ctx, fmt.Sprintf("/api/v1/team/objectives/%s/deliverables", goalID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		d, err := parseDeliverableCreate(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		result := "Deliverable created.\n\n" + formatDeliverableDetail(d.deliverable)
		if d.uploadURL != "" {
			filePath := argStr(args, "file_path")
			if filePath != "" {
				if err := uploadFile(ctx, d.uploadURL, d.uploadValues, filePath); err != nil {
					result += fmt.Sprintf("\n⚠ File upload failed: %v", err)
				} else {
					result += "\nFile uploaded successfully."
				}
			} else {
				result += fmt.Sprintf("\nUpload URL: %s\nUpload the file with: curl -X PUT '%s' --upload-file <filepath>", d.uploadURL, d.uploadURL)
			}
		}
		return textResult(result), nil
	}
}

func deleteDeliverableTool() mcp.Tool {
	return mcp.NewTool("delete_deliverable",
		mcp.WithDescription("成果物を削除する。この操作は取り消せない。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("deliverable_id",
			mcp.Required(),
			mcp.Description("Deliverable ID (short ID)"),
		),
	)
}

func handleDeleteDeliverable(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		deliverableID, err := client.ids.Resolve(argStr(args, "deliverable_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		_, err = client.Delete(ctx, fmt.Sprintf("/api/v1/team/objectives/%s/deliverables/%s", goalID, deliverableID), nil)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Deliverable deleted."), nil
	}
}

// --- Parsing & formatting ---

type deliverableInfo struct {
	ID          string
	NodeType    string
	DisplayName string
	Content     string
	Author      string
	DownloadURL string
	LinkURL     string
	HasChildren bool
	ChildCount  int64
	CreatedAt   string
}

type deliverableCreateResult struct {
	deliverable  deliverableInfo
	uploadURL    string
	uploadValues map[string]string
}

func parseDeliverableList(data []byte, ids *ShortIDCache) ([]deliverableInfo, error) {
	var raw struct {
		Deliverables []map[string]any `json:"deliverables"`
	}
	if err := json.Unmarshal(unwrapData(data), &raw); err != nil {
		return nil, err
	}

	result := make([]deliverableInfo, 0, len(raw.Deliverables))
	for _, d := range raw.Deliverables {
		result = append(result, extractDeliverableInfo(d, ids))
	}
	return result, nil
}

func parseDeliverable(data []byte, ids *ShortIDCache) (deliverableInfo, error) {
	var raw map[string]any
	if err := json.Unmarshal(unwrapData(data), &raw); err != nil {
		return deliverableInfo{}, err
	}
	return extractDeliverableInfo(raw, ids), nil
}

func parseDeliverableCreate(data []byte, ids *ShortIDCache) (deliverableCreateResult, error) {
	var raw map[string]any
	if err := json.Unmarshal(unwrapData(data), &raw); err != nil {
		return deliverableCreateResult{}, err
	}

	result := deliverableCreateResult{
		deliverable: extractDeliverableInfo(raw, ids),
	}
	if upload, ok := raw["uploadRequest"].(map[string]any); ok {
		result.uploadURL, _ = upload["url"].(string)
		if values, ok := upload["values"].(map[string]any); ok {
			result.uploadValues = make(map[string]string, len(values))
			for k, v := range values {
				if s, ok := v.(string); ok {
					result.uploadValues[k] = s
				}
			}
		}
	}
	return result, nil
}

func extractDeliverableInfo(d map[string]any, ids *ShortIDCache) deliverableInfo {
	fullID, _ := d["id"].(string)
	nodeType, _ := d["nodeType"].(string)
	displayName, _ := d["displayName"].(string)
	content, _ := d["content"].(string)
	downloadURL, _ := d["downloadUrl"].(string)
	createdAt, _ := d["createdAt"].(string)
	hasChildren, _ := d["hasChildren"].(bool)
	linkURL := ""
	if lu, ok := d["linkUrl"].(string); ok {
		linkURL = lu
	}
	childCount := int64(0)
	if cc, ok := d["childrenCount"].(float64); ok {
		childCount = int64(cc)
	}

	author := ""
	if a, ok := d["author"].(map[string]any); ok {
		author, _ = a["name"].(string)
	}

	return deliverableInfo{
		ID:          ids.Shorten(fullID),
		NodeType:    nodeType,
		DisplayName: displayName,
		Content:     content,
		Author:      author,
		DownloadURL: downloadURL,
		LinkURL:     linkURL,
		HasChildren: hasChildren,
		ChildCount:  childCount,
		CreatedAt:   formatTime(createdAt),
	}
}

func nodeTypeIcon(nodeType string) string {
	switch nodeType {
	case "folder":
		return "[folder]"
	case "document":
		return "[doc]"
	case "file":
		return "[file]"
	case "link":
		return "[link]"
	default:
		return "[?]"
	}
}

func formatDeliverableList(deliverables []deliverableInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Deliverables (%d):\n\n", len(deliverables))
	for _, d := range deliverables {
		fmt.Fprintf(&sb, "[%s] %s %s", d.ID, nodeTypeIcon(d.NodeType), d.DisplayName)
		if d.Author != "" {
			fmt.Fprintf(&sb, " (@%s)", d.Author)
		}
		if d.HasChildren {
			fmt.Fprintf(&sb, " (%d items)", d.ChildCount)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatDeliverableDetail(d deliverableInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s %s\n", nodeTypeIcon(d.NodeType), d.DisplayName)
	fmt.Fprintf(&sb, "ID: %s | Type: %s", d.ID, d.NodeType)
	if d.Author != "" {
		fmt.Fprintf(&sb, " | Author: %s", d.Author)
	}
	sb.WriteString("\n")
	if d.LinkURL != "" {
		fmt.Fprintf(&sb, "Link: %s\n", d.LinkURL)
	}
	if d.DownloadURL != "" {
		fmt.Fprintf(&sb, "Download: %s\n", d.DownloadURL)
	}
	if d.CreatedAt != "" {
		fmt.Fprintf(&sb, "Created: %s\n", d.CreatedAt)
	}
	if d.Content != "" {
		fmt.Fprintf(&sb, "\nContent:\n%s\n", d.Content)
	}
	return sb.String()
}

// uploadFile uploads a local file to S3 using a presigned POST URL.
func uploadFile(_ context.Context, uploadURL string, values map[string]string, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Write form fields first (required by S3 presigned POST)
	for k, v := range values {
		if err := w.WriteField(k, v); err != nil {
			return fmt.Errorf("writing field %s: %w", k, err)
		}
	}
	// Add Content-Type field if not already in values (required by S3 policy)
	if _, ok := values["Content-Type"]; !ok {
		ct := detectContentType(filePath)
		if err := w.WriteField("Content-Type", ct); err != nil {
			return fmt.Errorf("writing Content-Type: %w", err)
		}
	}

	// Write file field last
	part, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, uploadURL, &buf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func detectContentType(filePath string) string {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
