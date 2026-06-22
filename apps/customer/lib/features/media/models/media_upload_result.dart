class MediaUploadResult {
  const MediaUploadResult({required this.id, required this.url});

  final String id;
  final String url;

  factory MediaUploadResult.fromJson(Map<String, dynamic> json) {
    return MediaUploadResult(
      id: json['id'] as String,
      url: json['url'] as String,
    );
  }
}
