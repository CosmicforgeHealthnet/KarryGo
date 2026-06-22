/// A single notification as shown in the in-app feed.
///
/// The REST feed (via the customer-service proxy) returns notification-service
/// message objects with Go field names (Title, Body, EventType, Status,
/// CreatedAt, Data). The realtime websocket pushes a lighter payload
/// (event_type/title/body/data). [AppNotification.fromJson] tolerates both
/// shapes so the same model backs the feed and live pushes.
class AppNotification {
  const AppNotification({
    required this.id,
    required this.eventType,
    required this.title,
    required this.body,
    required this.createdAt,
    this.data = const {},
  });

  final String id;
  final String eventType;
  final String title;
  final String body;
  final DateTime createdAt;
  final Map<String, dynamic> data;

  factory AppNotification.fromJson(Map<String, dynamic> json) {
    String pick(List<String> keys) {
      for (final key in keys) {
        final value = json[key];
        if (value is String && value.isNotEmpty) return value;
      }
      return '';
    }

    DateTime parsedCreatedAt() {
      final raw = json['CreatedAt'] ?? json['created_at'];
      if (raw is String) {
        return DateTime.tryParse(raw)?.toLocal() ?? DateTime.now();
      }
      return DateTime.now();
    }

    final rawData = json['Data'] ?? json['data'];
    return AppNotification(
      id: pick(['ID', 'id']),
      eventType: pick(['EventType', 'event_type']),
      title: pick(['Title', 'title']),
      body: pick(['Body', 'body']),
      createdAt: parsedCreatedAt(),
      data: rawData is Map ? Map<String, dynamic>.from(rawData) : const {},
    );
  }
}
