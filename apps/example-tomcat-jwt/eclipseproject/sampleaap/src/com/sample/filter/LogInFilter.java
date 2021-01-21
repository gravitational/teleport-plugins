package com.sample.filter;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.FileReader;
import java.io.IOException;

import java.util.Base64;
import java.util.Date;
import java.util.HashSet;
import java.util.Properties;
import java.util.Set;
import java.util.StringTokenizer;

import javax.servlet.Filter;
import javax.servlet.FilterChain;
import javax.servlet.FilterConfig;
import javax.servlet.ServletException;
import javax.servlet.ServletRequest;
import javax.servlet.ServletResponse;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.servlet.http.HttpSession;


import org.json.JSONArray;
import org.json.JSONObject;

final public class LogInFilter implements Filter {

	private static String LOGONOBJ = "logonObject";

	boolean debug = true;

	@Override
	public void init(FilterConfig filterConfig) throws ServletException {
		// TODO Auto-generated method stub

	}



	protected void populateLogin(String jwtToken, LoginObject logonObject) throws FileNotFoundException, IOException {
		
		File propertiesFiles = new File("/etc/sampleaap.properties");
		boolean testMode = false;
		Properties properties = new Properties();
		Set<String> validRolesSet = new HashSet<String>();
		if (propertiesFiles.exists()) {
			properties.load(new FileReader(propertiesFiles));

			testMode = new Boolean(properties.getProperty("testmode"));
			if (testMode) {
				jwtToken = properties.getProperty("testJWTToken");
			}

			String validRoles = properties.getProperty("validRoles");
			if (validRoles != null) {
				StringTokenizer st = new StringTokenizer(validRoles, ",");
				for (String key; st.hasMoreElements();) {
					key = st.nextToken();
					validRolesSet.add(key);

				}

			}

		} else {
			System.out.println(new Date()
					+ ": no sampleapp.properties file at /etc/sampleapp.properties.  Setting to valid user.");
			logonObject.validLogin = true;
		}

		if (jwtToken == null) {
			logonObject.name = "anonymous";
			logonObject.userid = "anonymous";
			logonObject.roleName = "Public User";
			logonObject.validLogin = true;

		} else {
			
			String[] split_string = jwtToken.split("\\.");
			String base64EncodedHeader = split_string[0];
			String base64EncodedBody = split_string[1];
			

			System.out.println(new java.util.Date()+":~~~~~~~~~ JWT Header ~~~~~~~");
//	        org.apache.commons.codec.binary.Base64 base64Url = new org.apache.commons.codec.binary.Base64(true);
			String header = new String(Base64.getDecoder().decode(base64EncodedHeader));
			System.out.println(new java.util.Date()+":JWT Header : " + header);

			System.out.println(new java.util.Date()+":~~~~~~~~~ JWT Body ~~~~~~~");
			String body = new String(Base64.getDecoder().decode(base64EncodedBody));
			System.out.println("JWT Body : " + body);

			JSONObject jsonObject = new JSONObject(body);

			final String username = jsonObject.getString("username");
			JSONArray array = jsonObject.getJSONArray("roles");
			for (int i = 0; i < array.length(); i++) {
				logonObject.roles.add(array.getString(i));

			}
			logonObject.name = username;
			if (properties.containsKey(username + "_name")) {
				logonObject.name = properties.getProperty(username + "_name");

			}
			logonObject.userid = username;
			logonObject.roleName = "";
			for (String key : logonObject.roles) {
				String rolename = key;
				if (properties.containsKey(key + "_rolename"))
					rolename = properties.getProperty(key + "_rolename");

				logonObject.roleName += rolename + " ";
				if(validRolesSet.contains(key))
				{logonObject.validLogin=true;
					
				}
			}
		}

	}

	@Override
	public void doFilter(final ServletRequest request, final ServletResponse response, FilterChain chain)
			throws IOException, ServletException {

		if (request instanceof HttpServletRequest) {

			final HttpServletRequest httpRequest = (HttpServletRequest) request;
			
			//ignore the error jsp
			if(!httpRequest.getRequestURI().contains("error.jsp"))
			{
			final HttpSession session = httpRequest.getSession();

			LoginObject loginObject = (LoginObject) session.getAttribute(LOGONOBJ);

			if (loginObject == null) {
				loginObject = new LoginObject();
				session.setAttribute(LOGONOBJ, loginObject);

			}

			if (!loginObject.validLogin) // the default
			{
				String token = httpRequest.getHeader("teleport-jwt-assertion");

				
				populateLogin(token, loginObject);

				if (!loginObject.validLogin)
				{
					((HttpServletResponse) (response)).sendRedirect("./error.jsp");
					return;
				}

			}
			}

		}
		chain.doFilter(request, response);

	}

	@Override
	public void destroy() {

	}

}
