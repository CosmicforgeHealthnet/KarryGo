import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/app/data/mock_dashboard_data.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/settings/state/settings_controller.dart';
import 'package:vendor/features/settings/ui/settings_screen.dart';
import 'package:vendor/features/settings/ui/settings_editor.dart';
import 'package:vendor/features/settings/ui/branch_hours_screen.dart';
import 'package:vendor/features/settings/ui/role_settings_screen.dart';
import 'package:vendor/features/settings/ui/team_settings_screen.dart';
import 'package:vendor/features/settings/ui/settings_verification_screen.dart';
import 'package:vendor/features/dashboard/ui/vendor_workspace_screen.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

void main() {
  test(
    'profile replacement preserves complete fields and rejects stale versions',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = SettingsController(r.backend, SettingsKind.profile);
      addTearDown(c.dispose);
      await c.load();
      final original = c.rows.single;
      final body = {
        for (final key in [
          'legal_name',
          'trading_name',
          'support_email',
          'support_phone',
          'currency',
          'timezone',
        ])
          key: original[key],
        'expected_version': original['version'],
      };
      body['trading_name'] = 'Updated kitchen';
      expect(
        await c.mutate('PATCH', c.detail, body, parameters: c.scope),
        isTrue,
      );
      expect(c.rows.single['trading_name'], 'Updated kitchen');
      expect(c.rows.single['legal_name'], original['legal_name']);
      expect(
        await c.mutate('PATCH', c.detail, body, parameters: c.scope),
        isFalse,
      );
      expect(c.stale, isTrue);
      await c.load();
      expect(c.stale, isFalse);
    },
  );
  test('paused business can resume and new branches remain Draft', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final c = SettingsController(r.backend, SettingsKind.profile);
    addTearDown(c.dispose);
    await c.load();
    expect(
      await c.mutate('POST', '${c.detail}/pause', {
        'expected_version': c.rows.single['version'],
      }, parameters: c.scope),
      isTrue,
    );
    expect(c.rows.single['status'], 'PAUSED');
    expect(
      await c.mutate('POST', '${c.detail}/resume', {
        'expected_version': c.rows.single['version'],
      }, parameters: c.scope),
      isTrue,
    );
    expect(c.rows.single['status'], 'ACTIVE');
    final b = SettingsController(r.backend, SettingsKind.branches);
    addTearDown(b.dispose);
    await b.load();
    expect(
      await b.mutate('POST', b.collection, {
        'brand_id': '',
        'display_name': 'New branch',
        'address': {
          'line1': '12 Admiralty Way',
          'line2': '',
          'city': 'Lagos',
          'region': 'Lagos',
          'postal_code': '',
          'country_code': 'NG',
        },
        'latitude': 6.43,
        'longitude': 3.42,
        'timezone': 'Africa/Lagos',
      }, parameters: b.scope),
      isTrue,
    );
    final row = b.rows.last;
    expect(row['status'], 'DRAFT');
    expect(
      await b.mutate('POST', '${b.detail}/resume', {
        'expected_version': row['version'],
      }, parameters: b.params(row)),
      isFalse,
    );
  });
  test(
    'hours use parent branch version while preferences use their own version',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = SettingsController(r.backend, SettingsKind.branches);
      addTearDown(c.dispose);
      await c.load();
      final row = c.rows.first,
          params = c.params(c.rows.first),
          parent = row['version'];
      expect(
        await c.mutate('PUT', '${c.detail}/hours', {
          'expected_version': parent,
          'hours': [
            {
              'weekday': 1,
              'is_closed': false,
              'opens_at': '09:00',
              'closes_at': '12:00',
            },
            {
              'weekday': 1,
              'is_closed': false,
              'opens_at': '14:00',
              'closes_at': '18:00',
            },
            {'weekday': 2, 'is_closed': true},
          ],
        }, parameters: params),
        isTrue,
      );
      expect((await c.readRecord(row))['version'], (parent as int) + 1);
      const ops = <String, dynamic>{'version': 0};
      expect(
        await c.mutate('PUT', '${c.detail}/operations-settings', {
          'schema_version': 1,
          'accepting_new_orders': false,
          'pickup_enabled': true,
          'expected_version': ops['version'],
        }, parameters: params),
        isTrue,
      );
      expect(
        (await c.read(
          '${c.detail}/operations-settings',
          parameters: params,
        )).object['accepting_new_orders'],
        isFalse,
      );
      expect((await c.readRecord(row))['version'], parent + 1);
    },
  );
  test(
    'staff reads expose access records and exact assignment actions',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = SettingsController(r.backend, SettingsKind.staff);
      addTearDown(c.dispose);
      await c.load();
      expect(c.rows.first.containsKey('email'), isFalse);
      expect(c.rows.first.containsKey('name'), isFalse);
      expect(
        await c.mutate('POST', c.collection, {
          'email': 'new@example.com',
          'role_id': 'role_branch_manager',
          'branch_id': 'mock-branch',
        }),
        isTrue,
      );
      final staff = c.rows.last;
      expect(staff['status'], 'active');
      final assignments = SettingsController.objects(
        (await c.read('${c.detail}/roles', parameters: c.params(staff))).data,
        'assignments',
      );
      expect(assignments.single['branch_id'], 'mock-branch');
      expect(
        await c.mutate(
          'DELETE',
          '/api/v1/vendor/assignments/{id}',
          null,
          parameters: {'id': assignments.single['id'] as String},
        ),
        isTrue,
      );
      expect(
        await c.mutate('PUT', '${c.detail}/suspend', {
          'reason': 'Access review',
          'expected_version': staff['version'],
        }, parameters: c.params(staff)),
        isTrue,
      );
      final suspended = await c.readRecord(staff);
      expect(suspended['status'], 'suspended');
      expect(suspended['suspended_reason'], 'Access review');
      expect(
        await c.mutate('PUT', '${c.detail}/reactivate', {
          'expected_version': suspended['version'],
        }, parameters: c.params(staff)),
        isTrue,
      );
    },
  );
  test('high risk roles require a purpose bound single use grant', () async {
    final r = await dashboardRuntime();
    addTearDown(r.dispose);
    final c = SettingsController(r.backend, SettingsKind.roles);
    addTearDown(c.dispose);
    await c.load();
    expect(c.rows.first.containsKey('permission_codes'), isFalse);
    final catalogue = SettingsController.objects(
      (await c.read('/api/v1/vendor/permissions')).data,
      'permissions',
    );
    final risky = catalogue.firstWhere(
      (p) => p['is_high_risk'] == true,
    )['code'];
    final body = {
      'code': 'sensitive_access',
      'name': 'Sensitive access',
      'description': '',
      'permission_codes': [risky],
    };
    expect(await c.mutate('POST', c.collection, body), isFalse);
    final challenge = await r.auth.startStepUp(settingsStepUpPurpose);
    final grant = await r.auth.verifyStepUp(
      purpose: settingsStepUpPurpose,
      challengeId: challenge.challengeId,
      otp: '123456',
    );
    expect(
      await c.mutate('POST', c.collection, {
        ...body,
        'step_up_grant_id': grant.grantId,
      }),
      isTrue,
    );
    expect((await c.readRecord(c.rows.last))['permission_codes'], [risky]);
    expect(
      await c.mutate('POST', c.collection, {
        ...body,
        'code': 'second_role',
        'step_up_grant_id': grant.grantId,
      }),
      isFalse,
    );
  });
  test(
    'system roles are read only and settings denial leaves business reads usable',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final c = SettingsController(r.backend, SettingsKind.roles);
      addTearDown(c.dispose);
      await c.load();
      final row = await c.readRecord(c.rows.first);
      expect(
        await c.mutate('PUT', '${c.detail}/permissions', {
          'permission_codes': [],
          'expected_version': row['version'],
        }, parameters: c.params(row)),
        isFalse,
      );
      final restricted = await dashboardRuntime(
        scenario: MockDashboardScenario.settingsRestricted,
      );
      addTearDown(restricted.dispose);
      final team = SettingsController(restricted.backend, SettingsKind.staff);
      final profile = SettingsController(
        restricted.backend,
        SettingsKind.profile,
      );
      addTearDown(team.dispose);
      addTearDown(profile.dispose);
      await team.load();
      await profile.load();
      expect(team.restricted, isTrue);
      expect(profile.rows, isNotEmpty);
      expect(profile.stale, isFalse);
    },
  );
  test(
    'brand edits retain opaque references and branch links use business capability IDs',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final brands = SettingsController(r.backend, SettingsKind.brands);
      addTearDown(brands.dispose);
      await brands.load();
      expect(
        await brands.mutate('POST', brands.collection, {
          'display_name': 'Demo kitchen',
          'description': 'Prepared daily.',
          'media_refs': [],
        }, parameters: brands.scope),
        isTrue,
      );
      final original = brands.rows.first;
      expect(
        await brands.mutate('PATCH', brands.detail, {
          'display_name': 'Fresh kitchen',
          'description': 'Prepared daily.',
          'media_refs': [
            {'kind': 'LOGO', 'media_ref': 'media:existing-logo'},
          ],
          'expected_version': original['version'],
        }, parameters: brands.params(original)),
        isTrue,
      );
      expect(brands.rows.first['media_refs'], [
        {'kind': 'LOGO', 'media_ref': 'media:existing-logo'},
      ]);
      final caps = SettingsController(r.backend, SettingsKind.capabilities);
      addTearDown(caps.dispose);
      await caps.load();
      final types = SettingsController.objects(
        (await caps.read('/api/v1/vendor/capability-types')).data,
      );
      final next = types.firstWhere(
        (t) => !caps.rows.any((c) => c['capability_type'] == t['type']),
      );
      expect(
        await caps.mutate('POST', caps.collection, {
          'capability_type': next['type'],
        }, parameters: caps.scope),
        isTrue,
      );
      final capability = caps.rows.last;
      expect(capability['status'], 'DRAFT');
      final branches = SettingsController(r.backend, SettingsKind.branches);
      addTearDown(branches.dispose);
      await branches.load();
      final params = {
        ...branches.params(branches.rows.first),
        'capabilityId': capability['id'] as String,
      };
      expect(
        await branches.mutate(
          'PUT',
          '${branches.detail}/capabilities/{capabilityId}',
          {'expected_version': 0},
          parameters: params,
        ),
        isTrue,
      );
      final links = SettingsController.objects(
        (await branches.read(
          '${branches.detail}/capabilities',
          parameters: branches.params(branches.rows.first),
        )).data,
      );
      final link = links.firstWhere(
        (l) => l['vendor_capability_id'] == capability['id'],
      );
      expect(link['status'], 'DRAFT');
      expect(link['effective'], isFalse);
      expect(link['id'], isNot(capability['id']));
    },
  );
  testWidgets('paused activated business stays in workspace', (tester) async {
    final r = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(r.dispose);
    final c = SettingsController(r.backend, SettingsKind.profile);
    addTearDown(c.dispose);
    await tester.runAsync(() async {
      await c.load();
      await c.mutate('POST', '${c.detail}/pause', {
        'expected_version': c.rows.single['version'],
      }, parameters: c.scope);
    });
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: VendorWorkspaceScreen(runtime: r, onSettings: () {}),
      ),
    );
    await flush(tester);
    expect(find.text('Home'), findsWidgets);
    expect(tester.takeException(), isNull);
  });
  for (final kind in SettingsKind.values) {
    testWidgets('${kind.name} settings fits phone and larger text', (
      tester,
    ) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          builder: (context, child) => MediaQuery(
            data: MediaQuery.of(
              context,
            ).copyWith(textScaler: const TextScaler.linear(1.4)),
            child: child!,
          ),
          home: SettingsCollectionScreen(runtime: r, kind: kind),
        ),
      );
      await flush(tester);
      expect(tester.takeException(), isNull);
    });
  }
  testWidgets('hours, role and staff detail load without build notifications', (
    tester,
  ) async {
    final r = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(r.dispose);
    final b = SettingsController(r.backend, SettingsKind.branches);
    addTearDown(b.dispose);
    await tester.runAsync(b.load);
    final staff = SettingsController(r.backend, SettingsKind.staff);
    addTearDown(staff.dispose);
    await tester.runAsync(staff.load);
    final screens = [
      BranchHoursScreen(runtime: r, branch: b.rows.first),
      RolePermissionEditor(runtime: r),
      StaffSettingsScreen(runtime: r, record: staff.rows.last),
    ];
    for (final screen in screens) {
      await tester.pumpWidget(MaterialApp(theme: vendorTheme(), home: screen));
      await flush(tester);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    }
  });
  testWidgets(
    'hours interval sheet saves and stops loading before success feedback',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      final c = SettingsController(r.backend, SettingsKind.branches);
      addTearDown(c.dispose);
      await tester.runAsync(c.load);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: BranchHoursScreen(runtime: r, branch: c.rows.first),
        ),
      );
      await flush(tester);
      await tester.tap(find.byTooltip('Add Monday interval'));
      await tester.pumpAndSettle();
      expect(find.byType(BottomSheet), findsOneWidget);
      await tester.ensureVisible(find.text('Save interval'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Save interval'));
      await tester.pumpAndSettle();
      expect(find.text('09:00 - 18:00'), findsOneWidget);
      await tester.runAsync(() async {
        await tester.tap(find.text('Save hours'));
        await Future<void>.delayed(const Duration(milliseconds: 60));
      });
      await flush(tester);
      expect(find.text('Operating hours saved'), findsOneWidget);
      expect(find.byType(CircularProgressIndicator), findsNothing);
      expect(tester.takeException(), isNull);
      final hours = await tester.runAsync(
        () => c.read('${c.detail}/hours', parameters: c.params(c.rows.first)),
      );
      expect(SettingsController.objects(hours!.data).single['weekday'], 1);
      await tester.pumpWidget(const SizedBox());
    },
  );
  testWidgets('mock branch editor is prefilled and keyboard safe', (
    tester,
  ) async {
    final r = (await tester.runAsync(() => dashboardRuntime()))!;
    addTearDown(r.dispose);
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    tester.view.viewInsets = const FakeViewPadding(bottom: 300);
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    addTearDown(tester.view.resetViewInsets);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: SettingsEditor(
          runtime: r,
          kind: SettingsKind.branches,
          onSave: (_) async => true,
          errorMessage: () => null,
          canSave: () => true,
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Victoria Island branch'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
