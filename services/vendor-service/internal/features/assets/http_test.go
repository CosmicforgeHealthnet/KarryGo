package assets

import "testing"

func TestAssetUploadValidation(t *testing.T) {
	valid := uploadRequest{TargetType: "FOOD_ITEM", TargetRef: "item-a", FileName: "meal.jpg", MIMEType: "image/jpeg", ByteSize: 100}
	if !validUpload(valid) {
		t.Fatal("valid photo rejected")
	}
	for name, change := range map[string]func(*uploadRequest){"purpose": func(r *uploadRequest) { r.TargetType = "ORDER" }, "oversize": func(r *uploadRequest) { r.ByteSize = MaxBytes + 1 }, "empty": func(r *uploadRequest) { r.ByteSize = 0 }, "path": func(r *uploadRequest) { r.FileName = "../meal.jpg" }, "active content": func(r *uploadRequest) { r.MIMEType = "image/svg+xml" }, "product pdf": func(r *uploadRequest) { r.MIMEType = "application/pdf" }} {
		t.Run(name, func(t *testing.T) {
			input := valid
			change(&input)
			if validUpload(input) {
				t.Fatal("invalid upload accepted")
			}
		})
	}
	valid.TargetType = "PASTRY_REQUEST"
	valid.MIMEType = "application/pdf"
	valid.FileName = "reference.pdf"
	if !validUpload(valid) {
		t.Fatal("pastry PDF rejected")
	}
}
