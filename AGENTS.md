# MathMind — AI Agent Instructions

## Project Goal
Build an MVP math learning platform that teaches reasoning steps instead of final answers.

Target audience:
- Students: grades 5–7
- Teachers: math teachers
Language: Russian (RU)

## Core Concept
Each math task is represented as a **Decision Tree**:
- Student chooses a reasoning strategy
- Each choice leads to the next step or a mistake
- Mistakes are categorized by type (mistake_type)
- System provides explanation and hint per mistake

## MVP Scope
### Student
- Login
- Topic map (2 topics)
- Task flow with decision tree
- Feedback on mistakes
- Progress tracking

### Teacher
- Login
- Class overview
- Mistake analytics by concept
- Student profiles

## Technical Stack
Backend:
- Go
- REST API
- PostgreSQL
- Explicit SQL (no ORM)

Frontend:
- Next.js
- TypeScript
- Tailwind
- PWA enabled

Deployment:
- Docker
- docker-compose

## Required Features
1. docker-compose up starts full system
2. Student can complete tasks by choosing steps
3. Wrong choices return specific hint and mistake_type
4. Teacher dashboard shows aggregated mistakes
5. Seed data:
   - 2 concepts
   - 12 tasks total
   - Each task has 2–4 decision nodes

## Data Entities (conceptual)
- users (student, teacher)
- classes
- class_students
- concepts
- tasks
- task_nodes
- task_choices
- attempts
- attempt_steps

## Acceptance Criteria
- Project runs locally
- UI in Russian
- Mobile friendly
- Clean modular code
- README instructions complete

## Non-goals
- Payments
- Video lessons
- Full AI voice processing
