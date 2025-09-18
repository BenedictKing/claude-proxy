import { UpstreamConfig } from '../config/config'

export interface Provider {
  convertToProviderRequest(
    request: Request,
    baseUrl: string,
    apiKey: string,
    upstream?: UpstreamConfig
  ): Promise<Request>
  convertToClaudeResponse(providerResponse: Response): Promise<Response>
}