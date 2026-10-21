# GoTodo

GoTodo organizes work as tasks moving through a kanban board, with optional one-level relationships for tracking larger outcomes.

## Language

**Task**:
A card representing independently trackable work on a board.
_Avoid_: Ticket, issue

**Parent task**:
A top-level task that organizes direct subtasks while remaining independently trackable.
_Avoid_: Group, epic

**Subtask**:
A task belonging to one parent task. A subtask cannot itself contain subtasks.
_Avoid_: Subticket, child ticket

**Family**:
A parent task and all of its direct subtasks, viewed together across board columns.
_Avoid_: Group

**Terminal column**:
The board column representing completion: `done` on a personal board or `shipped` on a development board.
