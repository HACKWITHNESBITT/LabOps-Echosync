import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'models/echosync_models.dart';
import 'services/echosync_client.dart';

class RadarScreen extends StatefulWidget {
  final String userId;
  final String host;
  final int port;
  final EchoSyncClient? client;
  final EchoMatch? latestMatch;
  final int? matchLatencyMs;

  const RadarScreen({
    super.key,
    this.userId = 'usr_demo',
    this.host = kDefaultApiHost,
    this.port = kDefaultApiPort,
    this.client,
    this.latestMatch,
    this.matchLatencyMs,
  });

  @override
  State<RadarScreen> createState() => _RadarScreenState();
}

class _RadarScreenState extends State<RadarScreen>
    with SingleTickerProviderStateMixin {
  late final AnimationController radarController;
  late final EchoSyncClient client;
  StreamSubscription<WsFrame>? _sub;
  late EchoMatch? _injectedMatch;

  RadarState? radarState;
  EchoMatch? latestMatch;
  bool notificationVisible = false;
  bool linked = false;
  Timer? _notificationTimer;
  Timer? _presenceTimer;
  bool _ownsClient = false;
  int? _lastLatencyMs;

  static const _demoInterests = [
    'RUST PROGRAMMING',
    'FORMULA 1',
    'MACHINE LEARNING',
    'HIKING',
  ];

  @override
  void initState() {
    super.initState();

    radarController = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 4),
    )..repeat();

    _injectedMatch = widget.latestMatch;
    if (_injectedMatch != null) {
      latestMatch = _injectedMatch;
      notificationVisible = true;
    }

    _wireLive();
  }

  @override
  void didUpdateWidget(covariant RadarScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    // Parent (main.dart) is the single source of truth for matches when a
    // shared client is used — one haptic per detected proximity match.
    if (widget.latestMatch != null &&
        widget.latestMatch?.id != oldWidget.latestMatch?.id) {
      _onMatch(widget.latestMatch!, widget.matchLatencyMs);
    }
  }

  void _wireLive() {
    if (widget.client != null) {
      client = widget.client!;
      _ownsClient = false;
      _sub = client.frames.listen(
        _onFrame,
        onError: (_) {},
      );
    } else {
      client = EchoSyncClient(
        userId: widget.userId,
        host: widget.host,
        port: widget.port,
      );
      _ownsClient = true;
      client.frames.listen(
        _onFrame,
        onError: (_) {},
      );
      client.connect();

      // Standalone mode: beacon ourselves on the radar every few seconds.
      _presenceTimer = Timer.periodic(const Duration(seconds: 6), (_) {
        client.sendPresence(37.7749295, -122.4194155, _demoInterests);
      });
      client.sendPresence(37.7749295, -122.4194155, _demoInterests);
    }
  }

  void _onFrame(WsFrame frame) {
    switch (frame) {
      case HelloFrame():
        if (mounted) setState(() => linked = true);
      case RadarFrame(:final state):
        if (mounted) setState(() => radarState = state);
      case MatchFrame(:final match, :final latencyMs):
        // With a shared client the parent (main.dart) routes matches here
        // through didUpdateWidget; avoid double-processing the haptic.
        if (_ownsClient) _onMatch(match, latencyMs);
      case PingFrame():
      case WsParseError():
        break;
    }
  }

  Future<void> _onMatch(EchoMatch match, int? latencyMs) async {
    if (!mounted) return;

    setState(() {
      latestMatch = match;
      _lastLatencyMs = latencyMs;
      notificationVisible = true;
    });

    // Native haptic trigger: proximity match -> physical cue (<500ms budget).
    await HapticFeedback.heavyImpact();
    if (mounted) {
      await HapticFeedback.mediumImpact();
      HapticFeedback.selectionClick();
    }

    _notificationTimer?.cancel();
    _notificationTimer = Timer(const Duration(seconds: 5), () {
      if (mounted) setState(() => notificationVisible = false);
    });
  }

  @override
  void dispose() {
    _notificationTimer?.cancel();
    _presenceTimer?.cancel();
    _sub?.cancel();
    if (_ownsClient) client.dispose();
    radarController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Color(0xFF121A29),
            Color(0xFF17243A),
            Color(0xFF17243A),
          ],
        ),
      ),

      child: SafeArea(
        child: Column(
          children: [
            // Pink Figma line
            Container(
              height: 2,
              width: double.infinity,
              color: const Color(0xFFE7839A),
            ),

            // Header
            Container(
              height: 62,
              padding: const EdgeInsets.symmetric(horizontal: 24),
              decoration: const BoxDecoration(
                border: Border(
                  bottom: BorderSide(color: Color(0xFF657187), width: 1),
                ),
              ),

              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  const Text(
                    'EchoSync',
                    style: TextStyle(
                      fontSize: 19,
                      fontWeight: FontWeight.bold,
                      color: Colors.white,
                    ),
                  ),
                  Row(
                    children: [
                      Container(
                        width: 7,
                        height: 7,
                        decoration: const BoxDecoration(
                          color: Color(0xFFE8899D),
                          shape: BoxShape.circle,
                        ),
                      ),
                      const SizedBox(width: 7),
                      Text(
                        linked ? 'linked' : 'connecting…',
                        style: const TextStyle(
                          fontSize: 14,
                          color: Color(0xFFE0E5ED),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),

            // Radar
            Expanded(
              child: Stack(
                children: [
                  Center(
                    child: AnimatedBuilder(
                      animation: radarController,
                      builder: (context, child) {
                        return CustomPaint(
                          size: const Size(330, 390),
                          painter: RadarPainter(
                            progress: radarController.value,
                            targets: radarState?.targets ?? const [],
                            hasMatches: latestMatch != null,
                          ),
                        );
                      },
                    ),
                  ),

                  // Match card
                  Positioned(
                    left: 28,
                    right: 28,
                    bottom: 18,
                    child: RadarMatchCard(match: latestMatch),
                  ),

                  // Live match notification (with haptic)
                  if (notificationVisible && latestMatch != null)
                    Positioned(
                      left: 24,
                      right: 24,
                      top: 16,
                      child: MatchNotification(
                        match: latestMatch!,
                        latencyMs: _lastLatencyMs,
                        onClose: () {
                          setState(() => notificationVisible = false);
                        },
                      ),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ============================================================
// MATCH NOTIFICATION
// ============================================================

class MatchNotification extends StatelessWidget {
  final EchoMatch match;
  final int? latencyMs;
  final VoidCallback onClose;

  const MatchNotification({
    super.key,
    required this.match,
    this.latencyMs,
    required this.onClose,
  });

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,

      child: Container(
        padding: const EdgeInsets.all(14),

        decoration: BoxDecoration(
          color: const Color(0xFF202D40),
          border: Border.all(color: const Color(0xFF7C8A9D), width: .8),
          borderRadius: BorderRadius.circular(4),
          boxShadow: [
            BoxShadow(color: Colors.black.withOpacity(.35), blurRadius: 18),
          ],
        ),

        child: Row(
          children: [
            Container(
              width: 9,
              height: 9,
              decoration: const BoxDecoration(
                color: Color(0xFFE8899D),
                shape: BoxShape.circle,
              ),
            ),

            const SizedBox(width: 12),

            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Match nearby',
                    style: TextStyle(fontSize: 13, fontWeight: FontWeight.w700),
                  ),

                  const SizedBox(height: 4),

                  Text(
                    '${match.sharedTokens.firstOrNull ?? match.sharedTokens.take(1).join()} · ~${match.distanceM.round()} m',
                    style: const TextStyle(fontSize: 11, color: Color(0xFFB5C0D0)),
                  ),
                ],
              ),
            ),

            if (latencyMs != null)
              Text(
                '${latencyMs}ms',
                style: const TextStyle(fontSize: 9, color: Color(0xFF9AA6B7)),
              ),

            const SizedBox(width: 8),

            GestureDetector(
              onTap: onClose,
              child: const Icon(Icons.close, size: 17, color: Color(0xFF9AA6B7)),
            ),
          ],
        ),
      ),
    );
  }
}

// ============================================================
// RADAR MATCH CARD
// ============================================================

class RadarMatchCard extends StatelessWidget {
  final EchoMatch? match;

  const RadarMatchCard({super.key, this.match});

  @override
  Widget build(BuildContext context) {
    final m = match;
    final interest = m?.sharedTokens.firstOrNull ?? 'Rust Programming';
    final peer = m?.peerUserId ?? 'USR_7x2';
    final dist = m?.distanceM ?? 6.0;

    return Container(
      padding: const EdgeInsets.only(top: 14),

      decoration: const BoxDecoration(
        border: Border(top: BorderSide(color: Color(0xFF778296), width: 1)),
      ),

      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            m == null
                ? 'scanning for shared interests…'
                : 'haptic triggered · match nearby',
            style: const TextStyle(fontSize: 11, color: Color(0xFF9BA5B5)),
          ),

          const SizedBox(height: 8),

          Text(
            interest,
            style: const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),

          const SizedBox(height: 4),

          Text(
            '$peer · ~${dist.round()} m away',
            style: const TextStyle(fontSize: 11, color: Color(0xFF9BA5B5)),
          ),
        ],
      ),
    );
  }
}

// ============================================================
// RADAR PAINTER
// ============================================================

class RadarPainter extends CustomPainter {
  final double progress;
  final List<RadarTarget> targets;
  final bool hasMatches;

  RadarPainter({
    required this.progress,
    this.targets = const [],
    this.hasMatches = false,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    const radius = 130.0;

    // Glow
    final glowPaint = Paint()
      ..shader = RadialGradient(
        colors: [
          const Color(0xFFE7849B).withOpacity(.14),
          Colors.transparent,
        ],
      ).createShader(Rect.fromCircle(center: center, radius: radius));

    canvas.drawCircle(center, radius, glowPaint);

    // Rings
    final circlePaint = Paint()
      ..color = const Color(0xFF9BAAC1).withOpacity(.45)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;

    for (int i = 1; i <= 4; i++) {
      canvas.drawCircle(center, radius * i / 4, circlePaint);
    }

    // Grid
    final gridPaint = Paint()
      ..color = const Color(0xFF9BAAC1).withOpacity(.28)
      ..strokeWidth = 1;

    canvas.drawLine(Offset(center.dx - radius, center.dy),
        Offset(center.dx + radius, center.dy), gridPaint);
    canvas.drawLine(Offset(center.dx, center.dy - radius),
        Offset(center.dx, center.dy + radius), gridPaint);
    canvas.drawLine(
        Offset(center.dx - radius * .7, center.dy - radius * .7),
        Offset(center.dx + radius * .7, center.dy + radius * .7), gridPaint);
    canvas.drawLine(
        Offset(center.dx - radius * .7, center.dy + radius * .7),
        Offset(center.dx + radius * .7, center.dy - radius * .7), gridPaint);

    // Scanning beam
    final angle = progress * math.pi * 2;
    final end = Offset(
      center.dx + math.cos(angle) * radius,
      center.dy + math.sin(angle) * radius,
    );
    final scanPaint = Paint()
      ..color = const Color(0xFFE990A5)
      ..strokeWidth = 1.2;
    canvas.drawLine(center, end, scanPaint);

    // Center
    canvas.drawCircle(center, 7, Paint()..color = const Color(0xFFE78A9F));
    canvas.drawCircle(center, 3, Paint()..color = const Color(0xFF182334));

    // Live targets: placed by the server (geo -> radar x/y link endpoint) and
    // connected with a dotted vector link, exactly like the web command center.
    final matched = hasMatches ? Color(0xFFE78A9F) : const Color(0xFFE2E8F0);
    for (final t in targets) {
      final link = t.link.to;
      final lx = link.isNotEmpty ? link[0] : 0.0;
      final ly = link.length > 1 ? link[1] : 0.0;
      // scale server space (radius ~120) into canvas radius 130
      final dx = (lx * radius / 120.0).clamp(-radius, radius).toDouble();
      final dy = (ly * radius / 120.0).clamp(-radius, radius).toDouble();
      final targetPos = Offset(center.dx + dx, center.dy + dy);

      // Dotted vector link
      final distance = (targetPos - center).distance;
      if (distance > 1) {
        final direction = (targetPos - center) / distance;
        final dashPaint = Paint()
          ..color = const Color(0xFFE1E6ED)
          ..strokeWidth = 1;
        for (double d = 15; d < distance - 20; d += 10) {
          canvas.drawLine(center + direction * d, center + direction * (d + 5),
              dashPaint);
        }
      }

      // Target halo + core
      final targetRing = Paint()
        ..color = matched
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5;
      canvas.drawCircle(targetPos, 23, targetRing);
      canvas.drawCircle(targetPos, 9, Paint()..color = matched);
      canvas.drawCircle(
          targetPos, 4, Paint()..color = const Color(0xFF667387));
    }
  }

  @override
  bool shouldRepaint(covariant RadarPainter oldDelegate) {
    return oldDelegate.progress != progress ||
        oldDelegate.targets != targets ||
        oldDelegate.hasMatches != hasMatches;
  }
}

extension<T> on List<T> {
  T? get firstOrNull => isEmpty ? null : first;
}