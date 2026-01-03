import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

void main() {
  runApp(const MathMindApp());
}

class MathMindApp extends StatefulWidget {
  const MathMindApp({super.key});

  @override
  State<MathMindApp> createState() => _MathMindAppState();
}

class _MathMindAppState extends State<MathMindApp> {
  int _tabIndex = 0;
  final api = ApiClient();

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      title: 'MathMind',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.indigo),
        scaffoldBackgroundColor: const Color(0xfff7f7fb),
        useMaterial3: true,
      ),
      home: Scaffold(
        appBar: AppBar(
          title: const Text('MathMind — шаги решения'),
          backgroundColor: Colors.indigo,
          foregroundColor: Colors.white,
        ),
        body: SafeArea(
          child: IndexedStack(
            index: _tabIndex,
            children: [
              StudentFlow(api: api),
              TeacherFlow(api: api),
            ],
          ),
        ),
        bottomNavigationBar: NavigationBar(
          selectedIndex: _tabIndex,
          onDestinationSelected: (index) => setState(() => _tabIndex = index),
          destinations: const [
            NavigationDestination(icon: Icon(Icons.school_outlined), label: 'Ученик'),
            NavigationDestination(icon: Icon(Icons.person_pin), label: 'Учитель'),
          ],
        ),
      ),
    );
  }
}

class ApiClient {
  ApiClient({String? baseUrl}) : baseUrl = baseUrl ?? 'http://localhost:8080';

  final String baseUrl;

  Future<ApiResult> loginTeacher(String email, String password) async {
    final uri = Uri.parse('$baseUrl/auth/teacher/login');
    return _post(uri, {'email': email, 'password': password});
  }

  Future<ApiResult> createClass(int teacherId, String name) async {
    final uri = Uri.parse('$baseUrl/classes');
    return _post(uri, {'teacher_id': teacherId, 'name': name});
  }

  Future<ApiResult> joinClass(String name, String invite) async {
    final uri = Uri.parse('$baseUrl/classes/join');
    return _post(uri, {'student_name': name, 'invite_code': invite});
  }

  Future<ApiResult> fetchTopics() async {
    final uri = Uri.parse('$baseUrl/topics');
    return _get(uri);
  }

  Future<ApiResult> fetchTasks(int topicId) async {
    final uri = Uri.parse('$baseUrl/tasks?topic_id=$topicId');
    return _get(uri);
  }

  Future<ApiResult> sendStep({
    required int? attemptId,
    required int studentId,
    required int classId,
    required int taskId,
    required String nodeKey,
    required String choiceKey,
  }) async {
    final uri = Uri.parse('$baseUrl/attempts');
    return _post(uri, {
      'attempt_id': attemptId,
      'student_id': studentId,
      'class_id': classId,
      'task_id': taskId,
      'node_key': nodeKey,
      'choice_key': choiceKey,
    });
  }

  Future<ApiResult> fetchAnalytics(int classId) async {
    final uri = Uri.parse('$baseUrl/teacher/analytics?class_id=$classId');
    return _get(uri);
  }

  Future<ApiResult> requestAI({
    required int teacherId,
    required int classId,
    required int topicId,
    required String type,
    required String goal,
  }) async {
    final uri = Uri.parse('$baseUrl/teacher/ai');
    return _post(uri, {
      'teacher_id': teacherId,
      'class_id': classId,
      'topic_id': topicId,
      'type': type,
      'goal': goal,
    });
  }

  Future<ApiResult> _post(Uri uri, Map<String, dynamic> body) async {
    try {
      final resp = await http.post(uri,
          headers: {'Content-Type': 'application/json'},
          body: jsonEncode(body));
      return ApiResult.fromResponse(resp);
    } catch (e) {
      return ApiResult.offline('Не удалось достучаться до сервера: $e');
    }
  }

  Future<ApiResult> _get(Uri uri) async {
    try {
      final resp = await http.get(uri);
      return ApiResult.fromResponse(resp);
    } catch (e) {
      return ApiResult.offline('Не удалось получить данные: $e');
    }
  }
}

class ApiResult {
  ApiResult({required this.ok, required this.statusCode, this.data, this.error, this.offlineMessage});

  final bool ok;
  final int statusCode;
  final Map<String, dynamic>? data;
  final String? error;
  final String? offlineMessage;

  factory ApiResult.fromResponse(http.Response resp) {
    try {
      final parsed = jsonDecode(resp.body) as Map<String, dynamic>;
      if (resp.statusCode >= 200 && resp.statusCode < 300) {
        return ApiResult(ok: true, statusCode: resp.statusCode, data: parsed);
      }
      return ApiResult(ok: false, statusCode: resp.statusCode, error: parsed['error']?.toString());
    } catch (_) {
      return ApiResult(ok: false, statusCode: resp.statusCode, error: 'Ошибка разбора ответа');
    }
  }

  factory ApiResult.offline(String message) =>
      ApiResult(ok: false, statusCode: 0, offlineMessage: message);
}

class Topic {
  Topic({required this.id, required this.title, required this.summary, required this.tasksCount});

  final int id;
  final String title;
  final String summary;
  final int tasksCount;

  factory Topic.fromJson(Map<String, dynamic> json) => Topic(
        id: json['id'] as int,
        title: json['title'] as String,
        summary: json['summary'] as String,
        tasksCount: json['tasks_count'] as int? ?? 0,
      );
}

class Task {
  Task({required this.id, required this.title, required this.description, required this.rootNode, required this.nodes});

  final int id;
  final String title;
  final String description;
  final String rootNode;
  final Map<String, TaskNode> nodes;

  factory Task.fromJson(Map<String, dynamic> json) {
    final rawNodes = json['nodes'] as Map<String, dynamic>? ?? {};
    final nodes = <String, TaskNode>{};
    rawNodes.forEach((key, value) {
      nodes[key] = TaskNode.fromJson(value as Map<String, dynamic>);
    });
    return Task(
      id: json['id'] as int,
      title: json['title'] as String,
      description: json['description'] as String,
      rootNode: json['root_node'] as String? ?? 'start',
      nodes: nodes,
    );
  }
}

class TaskNode {
  TaskNode({required this.key, required this.prompt, required this.isTerminal, required this.choices});

  final String key;
  final String prompt;
  final bool isTerminal;
  final List<TaskChoice> choices;

  factory TaskNode.fromJson(Map<String, dynamic> json) {
    final list = (json['choices'] as List?) ?? [];
    return TaskNode(
      key: json['key'] as String,
      prompt: json['prompt'] as String,
      isTerminal: json['is_terminal'] as bool? ?? false,
      choices: list.map((c) => TaskChoice.fromJson(c as Map<String, dynamic>)).toList(),
    );
  }
}

class TaskChoice {
  TaskChoice({required this.key, required this.text, required this.nextNode, required this.isCorrect, required this.mistakeType, required this.hint});

  final String key;
  final String text;
  final String nextNode;
  final bool isCorrect;
  final String mistakeType;
  final String hint;

  factory TaskChoice.fromJson(Map<String, dynamic> json) => TaskChoice(
        key: json['key'] as String,
        text: json['text'] as String,
        nextNode: json['next_node_key'] as String? ?? '',
        isCorrect: json['is_correct'] as bool? ?? false,
        mistakeType: json['mistake_type'] as String? ?? '',
        hint: json['hint'] as String? ?? '',
      );
}

class StudentFlow extends StatefulWidget {
  const StudentFlow({super.key, required this.api});

  final ApiClient api;

  @override
  State<StudentFlow> createState() => _StudentFlowState();
}

class _StudentFlowState extends State<StudentFlow> {
  final _nameCtrl = TextEditingController(text: 'Саша');
  final _codeCtrl = TextEditingController(text: 'MATH7A');
  int? studentId;
  int? classId;
  List<Topic> topics = [];
  bool loadingTopics = false;
  String? error;

  Task? selectedTask;
  List<Task> tasks = [];
  bool loadingTasks = false;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _codeCtrl.dispose();
    super.dispose();
  }

  Future<void> _loadTopics() async {
    setState(() {
      loadingTopics = true;
      error = null;
    });
    final resp = await widget.api.fetchTopics();
    if (resp.ok && resp.data != null) {
      final list = (resp.data!['topics'] as List?) ?? [];
      setState(() {
        topics = list.map((e) => Topic.fromJson(e as Map<String, dynamic>)).toList();
      });
    } else {
      setState(() => error = resp.offlineMessage ?? resp.error ?? 'Не удалось загрузить темы');
    }
    setState(() => loadingTopics = false);
  }

  Future<void> _joinClass() async {
    final name = _nameCtrl.text.trim();
    final code = _codeCtrl.text.trim();
    if (name.isEmpty || code.isEmpty) {
      setState(() => error = 'Введите имя и код класса');
      return;
    }
    final resp = await widget.api.joinClass(name, code);
    if (resp.ok && resp.data != null) {
      setState(() {
        studentId = resp.data!['student_id'] as int?;
        classId = resp.data!['class_id'] as int?;
        error = null;
      });
      await _loadTopics();
    } else {
      setState(() => error = resp.offlineMessage ?? resp.error ?? 'Не удалось присоединиться к классу');
    }
  }

  Future<void> _loadTasks(int topicId) async {
    setState(() {
      loadingTasks = true;
      selectedTask = null;
    });
    final resp = await widget.api.fetchTasks(topicId);
    if (resp.ok && resp.data != null) {
      final list = (resp.data!['tasks'] as List?) ?? [];
      setState(() {
        tasks = list.map((e) => Task.fromJson(e as Map<String, dynamic>)).toList();
      });
    } else {
      setState(() => error = resp.offlineMessage ?? resp.error ?? 'Не удалось загрузить задания');
    }
    setState(() => loadingTasks = false);
  }

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Ученик', style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold)),
          const SizedBox(height: 8),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('Присоединиться к классу', style: TextStyle(fontWeight: FontWeight.w600)),
                  const SizedBox(height: 12),
                  TextField(
                    controller: _nameCtrl,
                    decoration: const InputDecoration(labelText: 'Имя ученика'),
                  ),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _codeCtrl,
                    decoration: const InputDecoration(labelText: 'Код класса'),
                  ),
                  const SizedBox(height: 12),
                  FilledButton.icon(
                    icon: const Icon(Icons.login),
                    onPressed: _joinClass,
                    label: const Text('Войти в класс'),
                  ),
                  if (studentId != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 8.0),
                      child: Text('Успех! ID ученика: $studentId, класс: $classId',
                          style: const TextStyle(color: Colors.green)),
                    ),
                ],
              ),
            ),
          ),
          if (error != null)
            Padding(
              padding: const EdgeInsets.only(top: 8.0),
              child: Text(error!, style: const TextStyle(color: Colors.red)),
            ),
          if (studentId != null) ...[
            const SizedBox(height: 8),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text('Темы', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
                TextButton.icon(
                  onPressed: loadingTopics ? null : _loadTopics,
                  icon: const Icon(Icons.refresh),
                  label: const Text('Обновить'),
                )
              ],
            ),
            if (loadingTopics)
              const Center(child: Padding(padding: EdgeInsets.all(12), child: CircularProgressIndicator()))
            else if (topics.isEmpty)
              const Text('Пока нет тем. Создайте их в базе или загрузите сиды.'),
            ...topics.map((t) => Card(
                  child: ListTile(
                    title: Text(t.title),
                    subtitle: Text(t.summary),
                    trailing: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        const Icon(Icons.playlist_add_check),
                        Text('${t.tasksCount} заданий'),
                      ],
                    ),
                    onTap: () => _loadTasks(t.id),
                  ),
                )),
            if (tasks.isNotEmpty) ...[
              const SizedBox(height: 12),
              const Text('Задания', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
              ...tasks.map((task) => Card(
                    child: ListTile(
                      title: Text(task.title),
                      subtitle: Text(task.description),
                      onTap: () => setState(() => selectedTask = task),
                      trailing: const Icon(Icons.chevron_right),
                    ),
                  )),
            ],
            if (selectedTask != null)
              TaskRunner(
                api: widget.api,
                classId: classId!,
                studentId: studentId!,
                task: selectedTask!,
              ),
          ]
        ],
      ),
    );
  }
}

class TaskRunner extends StatefulWidget {
  const TaskRunner({super.key, required this.api, required this.studentId, required this.classId, required this.task});

  final ApiClient api;
  final int studentId;
  final int classId;
  final Task task;

  @override
  State<TaskRunner> createState() => _TaskRunnerState();
}

class _TaskRunnerState extends State<TaskRunner> {
  late String currentNodeKey = widget.task.rootNode;
  int? attemptId;
  String? hint;
  String? message;
  bool completed = false;

  Future<void> _choose(TaskChoice choice) async {
    if (completed) return;
    final resp = await widget.api.sendStep(
      attemptId: attemptId,
      studentId: widget.studentId,
      classId: widget.classId,
      taskId: widget.task.id,
      nodeKey: currentNodeKey,
      choiceKey: choice.key,
    );

    if (resp.ok && resp.data != null) {
      final data = resp.data!;
      setState(() {
        attemptId = data['attempt_id'] as int? ?? attemptId;
        completed = data['completed'] as bool? ?? false;
        message = data['is_correct'] == true
            ? 'Верно!'
            : 'Есть неточность';
        hint = data['hint'] as String?;
        final next = data['next_node_key'] as String? ?? '';
        if (next.isNotEmpty) {
          currentNodeKey = next;
        } else if (choice.nextNode.isNotEmpty) {
          currentNodeKey = choice.nextNode;
        }
      });
    } else {
      // офлайн fallback: просто идём по локальным данным
      setState(() {
        completed = choice.isCorrect && (choice.nextNode.isEmpty ||
            (widget.task.nodes[choice.nextNode]?.isTerminal ?? false));
        message = choice.isCorrect ? 'Верно!' : 'Попробуй ещё раз';
        hint = choice.hint.isNotEmpty ? choice.hint : resp.offlineMessage;
        if (choice.nextNode.isNotEmpty) {
          currentNodeKey = choice.nextNode;
        }
      });
    }
  }

  void _reset() {
    setState(() {
      currentNodeKey = widget.task.rootNode;
      hint = null;
      message = null;
      completed = false;
      attemptId = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    final node = widget.task.nodes[currentNodeKey];
    if (node == null) {
      return const SizedBox.shrink();
    }

    return Card(
      margin: const EdgeInsets.only(top: 12),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(widget.task.title,
                style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
            const SizedBox(height: 6),
            Text(node.prompt, style: const TextStyle(fontSize: 16)),
            if (message != null)
              Padding(
                padding: const EdgeInsets.only(top: 8.0),
                child: Text(message!, style: TextStyle(color: completed ? Colors.green : Colors.orange)),
              ),
            if (hint != null)
              Padding(
                padding: const EdgeInsets.only(top: 8.0),
                child: Text('Подсказка: $hint', style: const TextStyle(color: Colors.blueGrey)),
              ),
            const SizedBox(height: 8),
            if (completed)
              Row(
                children: [
                  FilledButton(onPressed: _reset, child: const Text('Пройти снова')),
                  const SizedBox(width: 8),
                  const Text('Задание завершено!', style: TextStyle(color: Colors.green)),
                ],
              )
            else
              Column(
                children: node.choices
                    .map((c) => Padding(
                          padding: const EdgeInsets.symmetric(vertical: 4.0),
                          child: FilledButton.tonal(
                            style: FilledButton.styleFrom(minimumSize: const Size.fromHeight(48)),
                            onPressed: () => _choose(c),
                            child: Align(
                              alignment: Alignment.centerLeft,
                              child: Text(c.text),
                            ),
                          ),
                        ))
                    .toList(),
              ),
          ],
        ),
      ),
    );
  }
}

class TeacherFlow extends StatefulWidget {
  const TeacherFlow({super.key, required this.api});

  final ApiClient api;

  @override
  State<TeacherFlow> createState() => _TeacherFlowState();
}

class _TeacherFlowState extends State<TeacherFlow> {
  final _emailCtrl = TextEditingController(text: 'teacher@mathmind.ru');
  final _passCtrl = TextEditingController(text: 'parol123');
  final _classNameCtrl = TextEditingController(text: '7Б');
  final _analyticsClassCtrl = TextEditingController(text: '1');
  final _goalCtrl = TextEditingController(text: 'Домашка на тему процентов');
  int? teacherId;
  String? token;
  String? inviteCode;
  String? error;
  List<Map<String, dynamic>> summary = [];
  List<Map<String, dynamic>> recent = [];
  Map<String, dynamic>? aiDraft;

  @override
  void dispose() {
    _emailCtrl.dispose();
    _passCtrl.dispose();
    _classNameCtrl.dispose();
    _analyticsClassCtrl.dispose();
    _goalCtrl.dispose();
    super.dispose();
  }

  Future<void> _login() async {
    final resp = await widget.api.loginTeacher(_emailCtrl.text.trim(), _passCtrl.text);
    if (resp.ok && resp.data != null) {
      setState(() {
        teacherId = resp.data!['teacher_id'] as int?;
        token = resp.data!['token'] as String?;
        error = null;
      });
    } else {
      setState(() => error = resp.offlineMessage ?? resp.error);
    }
  }

  Future<void> _createClass() async {
    if (teacherId == null) {
      setState(() => error = 'Сначала войдите');
      return;
    }
    final resp = await widget.api.createClass(teacherId!, _classNameCtrl.text.trim());
    if (resp.ok && resp.data != null) {
      setState(() {
        inviteCode = resp.data!['invite_code'] as String?;
        _analyticsClassCtrl.text = (resp.data!['class_id']).toString();
      });
    } else {
      setState(() => error = resp.offlineMessage ?? resp.error);
    }
  }

  Future<void> _loadAnalytics() async {
    final id = int.tryParse(_analyticsClassCtrl.text.trim());
    if (id == null) {
      setState(() => error = 'Укажите ID класса');
      return;
    }
    final resp = await widget.api.fetchAnalytics(id);
    if (resp.ok && resp.data != null) {
      setState(() {
        summary = (resp.data!['mistake_summary'] as List?)?.cast<Map<String, dynamic>>() ?? [];
        recent = (resp.data!['recent_steps'] as List?)?.cast<Map<String, dynamic>>() ?? [];
        error = null;
      });
    } else {
      setState(() => error = resp.offlineMessage ?? resp.error);
    }
  }

  Future<void> _requestAI() async {
    if (teacherId == null) {
      setState(() => error = 'Сначала войдите');
      return;
    }
    final classId = int.tryParse(_analyticsClassCtrl.text) ?? 1;
    final resp = await widget.api.requestAI(
      teacherId: teacherId!,
      classId: classId,
      topicId: 1,
      type: 'lesson_plan',
      goal: _goalCtrl.text,
    );
    if (resp.ok && resp.data != null) {
      setState(() => aiDraft = resp.data);
    } else {
      setState(() => error = resp.offlineMessage ?? resp.error);
    }
  }

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Учитель', style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold)),
          const SizedBox(height: 8),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('Вход', style: TextStyle(fontWeight: FontWeight.w700)),
                  TextField(controller: _emailCtrl, decoration: const InputDecoration(labelText: 'Email')),
                  TextField(controller: _passCtrl, decoration: const InputDecoration(labelText: 'Пароль'), obscureText: true),
                  const SizedBox(height: 12),
                  FilledButton(onPressed: _login, child: const Text('Войти')),
                  if (teacherId != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 8.0),
                      child: Text('ID учителя: $teacherId', style: const TextStyle(color: Colors.green)),
                    )
                ],
              ),
            ),
          ),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('Создать класс', style: TextStyle(fontWeight: FontWeight.w700)),
                  TextField(controller: _classNameCtrl, decoration: const InputDecoration(labelText: 'Название класса')),
                  const SizedBox(height: 12),
                  FilledButton.icon(
                    icon: const Icon(Icons.class_),
                    onPressed: _createClass,
                    label: const Text('Создать'),
                  ),
                  if (inviteCode != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 8.0),
                      child: Text('Код приглашения: $inviteCode'),
                    )
                ],
              ),
            ),
          ),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('Аналитика по ошибкам', style: TextStyle(fontWeight: FontWeight.w700)),
                  TextField(
                    controller: _analyticsClassCtrl,
                    decoration: const InputDecoration(labelText: 'ID класса'),
                    keyboardType: TextInputType.number,
                  ),
                  const SizedBox(height: 8),
                  FilledButton.icon(
                    icon: const Icon(Icons.analytics),
                    onPressed: _loadAnalytics,
                    label: const Text('Показать'),
                  ),
                  const SizedBox(height: 12),
                  if (summary.isEmpty)
                    const Text('Нет данных: ученики ещё не решали задачи.'),
                  ...summary.map((row) => ListTile(
                        leading: const Icon(Icons.warning_amber_rounded, color: Colors.orange),
                        title: Text(row['topic_title']?.toString() ?? 'Тема'),
                        subtitle: Text('Ошибка: ${row['mistake_type'] ?? '—'}'),
                        trailing: Text('×${row['count']}'),
                      )),
                  if (recent.isNotEmpty)
                    Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Divider(),
                        const Text('Недавние ответы', style: TextStyle(fontWeight: FontWeight.w700)),
                        ...recent.take(8).map((row) => ListTile(
                              dense: true,
                              title: Text(row['task_title']?.toString() ?? ''),
                              subtitle: Text('Шаг: ${row['node_key']} · Ошибка: ${row['mistake_type'] ?? 'нет'}'),
                              trailing: Icon(row['is_correct'] == true ? Icons.check : Icons.close,
                                  color: row['is_correct'] == true ? Colors.green : Colors.red),
                            )),
                      ],
                    ),
                ],
              ),
            ),
          ),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('AI-помощник', style: TextStyle(fontWeight: FontWeight.w700)),
                  TextField(
                    controller: _goalCtrl,
                    minLines: 2,
                    maxLines: 4,
                    decoration: const InputDecoration(labelText: 'Опишите урок, домашку или тест'),
                  ),
                  const SizedBox(height: 8),
                  FilledButton.icon(
                    icon: const Icon(Icons.bolt),
                    onPressed: _requestAI,
                    label: const Text('Сгенерировать'),
                  ),
                  if (aiDraft != null) ...[
                    const SizedBox(height: 8),
                    Text(aiDraft!['title']?.toString() ?? 'Черновик',
                        style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
                    Text('Источник: ${aiDraft!['source']}'),
                    const SizedBox(height: 4),
                    ...(aiDraft!['sections'] as List? ?? [])
                        .map((s) => Padding(
                              padding: const EdgeInsets.only(bottom: 4),
                              child: Text('• $s'),
                            )),
                  ]
                ],
              ),
            ),
          ),
          if (error != null)
            Padding(
              padding: const EdgeInsets.only(top: 8.0),
              child: Text(error!, style: const TextStyle(color: Colors.red)),
            ),
        ],
      ),
    );
  }
}
