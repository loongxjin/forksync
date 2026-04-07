import { Button } from './components/ui/button'

function App(): React.ReactElement {
  return (
    <div className="dark min-h-screen p-8">
      <h1 className="text-2xl font-bold mb-4">ForkSync</h1>
      <p className="text-muted-foreground mb-4">Auto-sync fork repos with AI agent conflict resolution</p>
      <div className="flex gap-2">
        <Button variant="default">Default</Button>
        <Button variant="secondary">Secondary</Button>
        <Button variant="outline">Outline</Button>
        <Button variant="destructive">Destructive</Button>
      </div>
    </div>
  )
}

export default App
