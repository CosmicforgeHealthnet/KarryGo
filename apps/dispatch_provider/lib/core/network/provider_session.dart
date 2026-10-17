import 'dart:convert';

import 'package:flutter/widgets.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../config.dart';
import '../../features/offline/state/offline_updates_controller.dart';
import '../theme/theme_controller.dart';

class ProviderSession extends InheritedWidget {
  const ProviderSession({
    super.key,
    required this.client,
    required this.accessToken,
    required this.refreshProfile,
    this.openNotifications,
    this.openCurrentDelivery,
    this.openOpportunities,
    this.offlineUpdates,
    this.completedSetupSteps,
    this.themeController,
    required super.child,
  });

  final http.Client client;
  final String? Function() accessToken;
  final Future<void> Function() refreshProfile;
  final VoidCallback? openNotifications;
  final VoidCallback? openCurrentDelivery;
  final VoidCallback? openOpportunities;
  final OfflineUpdatesController? offlineUpdates;
  final Set<int> Function()? completedSetupSteps;
  final ThemeController? themeController;

  static ProviderSession of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<ProviderSession>()!;

  Future<Map<String, dynamic>> command(
    String path,
    Map<String, dynamic> body, {
    String? idempotencyKey,
  }) async {
    final response = await client
        .post(
          Uri.parse('${AppConfig.apiBaseUrl}$path'),
          headers: {
            'Authorization': 'Bearer ${accessToken() ?? ''}',
            'Content-Type': 'application/json',
            'Accept': 'application/json',
            if (idempotencyKey != null) 'Idempotency-Key': idempotencyKey,
          },
          body: jsonEncode(body),
        )
        .timeout(const Duration(seconds: 30));
    Map<String, dynamic> envelope;
    try {
      envelope = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message: 'The server returned an unreadable response. Please retry.',
      );
    }
    if (response.statusCode < 200 ||
        response.statusCode >= 300 ||
        envelope['success'] == false) {
      throw ApiException.fromErrorEnvelope(
        envelope,
        statusCode: response.statusCode,
      );
    }
    return Map<String, dynamic>.from(envelope['data'] as Map? ?? envelope);
  }

  @override
  bool updateShouldNotify(ProviderSession oldWidget) =>
      client != oldWidget.client;
}
