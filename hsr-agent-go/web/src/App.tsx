import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { Layout } from '@/components/layout/Layout'
import { ChatPage } from '@/pages/ChatPage'
import { CharactersPage } from '@/pages/CharactersPage'
import { CharacterDetailPage } from '@/pages/CharacterDetailPage'
import { LightconesPage } from '@/pages/LightconesPage'
import { LightconeDetailPage } from '@/pages/LightconeDetailPage'
import { RelicSetsPage } from '@/pages/RelicSetsPage'
import { RelicSetDetailPage } from '@/pages/RelicSetDetailPage'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<ChatPage />} />
          <Route path="/characters" element={<CharactersPage />} />
          <Route path="/characters/:id" element={<CharacterDetailPage />} />
          <Route path="/lightcones" element={<LightconesPage />} />
          <Route path="/lightcones/:id" element={<LightconeDetailPage />} />
          <Route path="/relic-sets" element={<RelicSetsPage />} />
          <Route path="/relic-sets/:id" element={<RelicSetDetailPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
