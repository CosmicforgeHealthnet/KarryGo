import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

/// Detail key holding the server's original, untranslated message.
const serverMessageDetailKey = 'server_message';

/// The server's original message, lower-cased, for matching in app logic.
/// User-facing text is rewritten by [friendlyServerMessage], so logic must
/// never match on [ApiException.message].
String serverMessageOf(ApiException e) =>
    (e.details[serverMessageDetailKey] as String? ?? e.message).toLowerCase();

/// Rewrites server wording that only makes sense to developers into plain
/// language. Anything not recognised is returned unchanged, because most
/// server messages are already written for users.
String friendlyServerMessage(String message) {
  final lower = message.toLowerCase();
  if (lower.contains('e.164') || lower.contains('e164')) {
    return 'Enter a valid Nigerian phone number.';
  }
  if (lower.contains('uuid')) {
    return 'Something you selected is no longer valid. Refresh and try again.';
  }
  if (lower.contains('rfc3339') || lower.contains('rfc 3339')) {
    return 'Enter a valid date and time.';
  }
  if (lower.contains('expected_version') ||
      lower.contains('version conflict') ||
      lower.contains('stale version')) {
    return 'This was changed somewhere else. Refresh and try again.';
  }
  if (lower.contains('idempotency')) {
    return 'This request was already sent. Refresh to see the latest result.';
  }
  if (lower.contains('step_up') || lower.contains('step-up')) {
    return 'Confirm it is you to continue.';
  }
  return message;
}
