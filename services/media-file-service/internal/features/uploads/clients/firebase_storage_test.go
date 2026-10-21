package uploadclients

import "testing"

func TestTokenURLEscapesThePathAsOneSegment(t *testing.T) {
	c := &FirebaseStorageClient{bucketName: "logistics-93b9f.firebasestorage.app"}
	got := c.tokenURL("media/driver-taxi-service/profile_photo/d1/a b.jpg", "tok-123")
	want := "https://firebasestorage.googleapis.com/v0/b/logistics-93b9f.firebasestorage.app/o/media%2Fdriver-taxi-service%2Fprofile_photo%2Fd1%2Fa%20b.jpg?alt=media&token=tok-123"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
