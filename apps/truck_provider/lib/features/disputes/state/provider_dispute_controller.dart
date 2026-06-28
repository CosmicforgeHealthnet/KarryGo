import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../earnings/models/earnings_models.dart';
import '../data/provider_support_api.dart';
import '../models/dispute_models.dart';

/// Drives the disputes flow: the feedback list, the log-dispute steps
/// (select transaction → select type → submit), and per-complaint live chat.
class ProviderDisputeController extends ChangeNotifier {
  ProviderDisputeController({
    required ProviderSupportApi api,
    required String? Function() accessToken,
  })  : _api = api,
        _accessToken = accessToken;

  final ProviderSupportApi _api;
  final String? Function() _accessToken;
  String? get _token => _accessToken();

  // ─── Feedback list ──────────────────────────────────────────────────────────
  List<Complaint> _complaints = const [];
  List<Complaint> get complaints => _complaints;
  bool _loading = false;
  bool get loading => _loading;
  String? _error;
  String? get error => _error;

  Future<void> loadComplaints() async {
    final token = _token;
    if (token == null) return;
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      _complaints = await _api.listComplaints(accessToken: token);
    } catch (e) {
      _error = _msg(e);
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  // ─── New-dispute selection ──────────────────────────────────────────────────
  EarningsTransaction? _selectedTransaction;
  EarningsTransaction? get selectedTransaction => _selectedTransaction;
  String? _selectedType;
  String? get selectedType => _selectedType;

  void startNewDispute() {
    _selectedTransaction = null;
    _selectedType = null;
    _submitError = null;
    _createdComplaint = null;
    notifyListeners();
  }

  void selectTransaction(EarningsTransaction txn) {
    _selectedTransaction = txn;
    notifyListeners();
  }

  void selectType(String type) {
    _selectedType = type;
    notifyListeners();
  }

  // ─── Submission ─────────────────────────────────────────────────────────────
  bool _submitting = false;
  bool get submitting => _submitting;
  String? _submitError;
  String? get submitError => _submitError;
  Complaint? _createdComplaint;
  Complaint? get createdComplaint => _createdComplaint;

  Future<Complaint?> submitDispute() async {
    final token = _token;
    final txn = _selectedTransaction;
    final type = _selectedType;
    if (token == null || txn == null || type == null) return null;

    _submitting = true;
    _submitError = null;
    notifyListeners();
    try {
      final reference = txn.bookingId.isNotEmpty ? txn.bookingId : txn.id;
      final complaint = await _api.createComplaint(
        accessToken: token,
        subject: type,
        description: 'Dispute on transaction ${txn.id}: $type',
        bookingReference: reference,
      );
      _createdComplaint = complaint;
      _complaints = [complaint, ..._complaints];
      return complaint;
    } catch (e) {
      _submitError = _msg(e);
      return null;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  // ─── Live chat ──────────────────────────────────────────────────────────────
  List<DisputeMessage> _messages = const [];
  List<DisputeMessage> get messages => _messages;
  bool _loadingMessages = false;
  bool get loadingMessages => _loadingMessages;
  bool _sending = false;
  bool get sending => _sending;

  Future<void> loadMessages(String complaintId) async {
    final token = _token;
    if (token == null) return;
    _loadingMessages = true;
    notifyListeners();
    try {
      _messages = await _api.listMessages(accessToken: token, id: complaintId);
    } catch (_) {
      // Leave existing messages; the chat screen still renders the composer.
    } finally {
      _loadingMessages = false;
      notifyListeners();
    }
  }

  Future<void> sendMessage(String complaintId, String content) async {
    final token = _token;
    if (token == null || content.trim().isEmpty) return;
    _sending = true;
    notifyListeners();
    try {
      final msg = await _api.sendMessage(accessToken: token, id: complaintId, content: content.trim());
      _messages = [..._messages, msg];
    } catch (_) {
    } finally {
      _sending = false;
      notifyListeners();
    }
  }

  String _msg(Object e) => e is ApiException ? e.message : e.toString();
}
