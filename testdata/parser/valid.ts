interface User {
  id: number;
  name: string;
  email: string;
}

type Result<T> = { ok: true; value: T } | { ok: false; error: string };

function getUser(id: number): User | undefined {
  const users: User[] = [];
  return users.find((u) => u.id === id);
}

class Repository<T extends { id: number }> {
  private items: T[] = [];

  add(item: T): void {
    this.items.push(item);
  }

  findById(id: number): T | undefined {
    return this.items.find((item) => item.id === id);
  }
}

export { getUser, Repository };
export type { User, Result };
