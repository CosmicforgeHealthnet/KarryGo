import '../../../core/data/driver_client.dart';

/// A support case: a reported problem, or the driver's chat with support.
class SupportCase {
  const SupportCase({
    required this.id,
    required this.subject,
    required this.description,
    required this.status,
    required this.unread,
    this.createdAt,
  });
  final String id;
  final String subject;
  final String description;
  final String status;
  final int unread;
  final DateTime? createdAt;

  bool get isOpen => status != 'resolved' && status != 'closed';

  factory SupportCase.fromJson(Map<String, dynamic> j) => SupportCase(
    id: (j['id'] as String?) ?? '',
    subject: (j['subject'] as String?) ?? '',
    description: (j['description'] as String?) ?? '',
    status: (j['status'] as String?) ?? '',
    unread: (j['unread_count'] as num?)?.toInt() ?? 0,
    createdAt: DateTime.tryParse((j['created_at'] as String?) ?? '')?.toLocal(),
  );
}

class SupportMessage {
  const SupportMessage({
    required this.content,
    required this.fromMe,
    this.createdAt,
  });
  final String content;
  final bool fromMe;
  final DateTime? createdAt;

  factory SupportMessage.fromJson(Map<String, dynamic> j) => SupportMessage(
    content: (j['content'] as String?) ?? '',
    // Support agents reply as the admin; anything else is the driver.
    fromMe: (j['sender_type'] as String?) != 'admin',
    createdAt: DateTime.tryParse((j['created_at'] as String?) ?? '')?.toLocal(),
  );
}

class Faq {
  const Faq({required this.title, required this.body});
  final String title;
  final String body;
}

/// support-dispute-service, as a taxi driver (`/taxi-provider/...`).
class SupportApi {
  SupportApi(this._client);
  final DriverClient _client;
  static const _me = '/taxi-provider';

  Future<List<Faq>> faqs() async =>
      asList(
            await _client.get('/support/faqs', query: {'audience': 'provider'}),
            'faqs',
          )
          .map(
            (j) => Faq(
              title: (j['title'] as String?) ?? '',
              body: (j['body'] as String?) ?? '',
            ),
          )
          .toList();

  Future<List<SupportCase>> cases() async => asList(
    await _client.get('$_me/complaints'),
    'complaints',
  ).map(SupportCase.fromJson).toList();

  Future<SupportCase> report({
    required String subject,
    required String description,
    String? tripId,
  }) async => SupportCase.fromJson(
    asMap(
      await _client.post('$_me/complaints', {
        'service_type': 'taxi',
        'subject': subject,
        'description': description,
        if (tripId != null && tripId.isNotEmpty) 'booking_reference': tripId,
      }),
    ),
  );

  /// Opens (or reuses) the driver's live chat with support.
  Future<SupportCase> startChat() async => SupportCase.fromJson(
    asMap(
      await _client.post('$_me/support-chat/start', {'service_type': 'taxi'}),
    ),
  );

  Future<List<SupportMessage>> messages(String caseId) async => asList(
    await _client.get('$_me/complaints/$caseId/messages'),
    'messages',
  ).map(SupportMessage.fromJson).toList();

  Future<void> send(String caseId, String content) =>
      _client.post('$_me/complaints/$caseId/messages', {'content': content});

  Future<void> markRead(String caseId) =>
      _client.post('$_me/complaints/$caseId/messages/read');

  /// Raises an emergency: support sees it at the top of their queue.
  Future<void> sos({required String description, double? lat, double? lng}) =>
      _client.post('$_me/sos', {
        'service_type': 'taxi',
        'description': description,
        'lat': ?lat,
        'lng': ?lng,
      });
}
