import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { api } from '@/api/client'
import type {
  Character,
  CharacterSkills,
  Modifier,
  Recommendation,
  Synergy,
  TeamPlan,
} from '@/api/types'
import { Card, CardContent } from '@/components/ui/card'
import { CharacterHero } from '@/components/character/detail/CharacterHero'
import { Section } from '@/components/character/detail/Section'
import { AxesSection } from '@/components/character/detail/AxesSection'
import { SkillsSection } from '@/components/character/detail/SkillsSection'
import { RecommendSection } from '@/components/character/detail/RecommendSection'
import { SynergyList, TeamList } from '@/components/character/detail/SynergyTeams'
import { ModifierTable } from '@/components/character/detail/ModifierTable'

interface DetailData {
  char: Character
  skills: CharacterSkills | null
  synergies: Synergy[]
  teams: TeamPlan[]
  lightcones: Recommendation[]
  relics: Recommendation[]
  modifiers: Modifier[]
}

export function CharacterDetailPage() {
  const { id } = useParams<{ id: string }>()
  const charId = Number(id)
  const [data, setData] = useState<DetailData | null>(null)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!charId) return
    let cancelled = false
    setLoading(true)
    setErr(null)
    setData(null)

    // 详情是硬依赖,其余区块允许失败(Promise.allSettled)
    api
      .getCharacter(charId)
      .then(async (char) => {
        const [skills, synergies, teams, lightcones, relics, modifiers] =
          await Promise.allSettled([
            api.getCharacterSkills(charId),
            api.getCharacterSynergies(charId),
            api.getCharacterTeams(charId),
            api.getCharacterLightcones(charId),
            api.getCharacterRelics(charId),
            api.getCharacterModifiers(charId),
          ])
        if (cancelled) return
        const ok = <T,>(r: PromiseSettledResult<T[]>): T[] =>
          r.status === 'fulfilled' ? r.value : []
        setData({
          char,
          skills: skills.status === 'fulfilled' ? skills.value : null,
          synergies: ok(synergies),
          teams: ok(teams),
          lightcones: ok(lightcones),
          relics: ok(relics),
          modifiers: ok(modifiers),
        })
      })
      .catch((e) => {
        if (!cancelled) setErr(e.message ?? '加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [charId])

  return (
    <div className="mx-auto h-full w-full max-w-4xl overflow-y-auto px-4 py-5">
      <Link
        to="/characters"
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="size-4" />
        返回角色列表
      </Link>

      {loading ? (
        <p className="text-sm text-muted-foreground">加载中…</p>
      ) : err ? (
        <p className="text-sm text-destructive">加载失败：{err}</p>
      ) : data ? (
        <div className="space-y-7 pb-10">
          <Card>
            <CardContent>
              <CharacterHero char={data.char} />
            </CardContent>
          </Card>

          <Section title="能力轴">
            <AxesSection
              provides={data.char.axes?.provides ?? []}
              needs={data.char.axes?.needs ?? []}
            />
          </Section>

          {data.skills && (
            <Section title="技能 / 星魂">
              <SkillsSection data={data.skills} />
            </Section>
          )}

          <Section title="推荐配装">
            <RecommendSection lightcones={data.lightcones} relics={data.relics} />
          </Section>

          <div className="grid gap-7 lg:grid-cols-2">
            <Section title="协同角色" count={data.synergies.length}>
              <SynergyList synergies={data.synergies} />
            </Section>
            <Section title="推荐配队" count={data.teams.length}>
              <TeamList teams={data.teams} />
            </Section>
          </div>

          <ModifierTable modifiers={data.modifiers} />
        </div>
      ) : null}
    </div>
  )
}
