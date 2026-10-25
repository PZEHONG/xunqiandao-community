// HTTP DTOs verified against web/auth.go, official_auth.go and handlers.go.
export interface PlatformUser { id: number; username: string; student_number?: string; email?: string }
export interface Binding { account: string; school_id: string; school_name: string }
export interface SelfResponse { user: PlatformUser; bindings: Binding[]; consent_accepted: boolean; agreement_version: string }
export interface AuthStatus { agreement_version: string; official_login?: boolean; registration?: boolean; email_login?: boolean; session_days?: number }
export interface School { school_id: string; school_name: string }
export interface SignRecord { sign_time?: string | number; sign_time_text?: string; status_code?: string | number; status_code_text?: string; remark_text?: string; address_name?: string; address?: string; comment?: string; [key: string]: unknown }
export interface SignSnapshot { month_date: string; records: SignRecord[]; continuous_sign_in?: string | number; continuous_days?: string | number; user_name?: string; school_name?: string; class_name?: string; [key: string]: unknown }
export interface QueryResponse { message?: string; data: SignSnapshot }
