<?xml version="1.0" encoding="ISO-8859-1"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"  
                xmlns:msxsl="urn:schemas-microsoft-com:xslt"
                xmlns:user="http://android.riteshsahu.com">
<xsl:template match="/">

<html>
	<head>
		<style type="text/css">
		body 
		{
			font-family:arial,sans-serif;
			color:#000;
			font-size:13px;
			color:#333;
		}
		table 
		{
			font-size:1em;
			margin:0 0 1em;
			border-collapse:collapse;
			border-width:0;
			empty-cells:show;
		}
		td,th 
		{
			border:1px solid #ccc;
			padding:6px 12px;
			text-align:left;
			vertical-align:top;
			background-color:inherit;
		}
		th 
		{
			background-color:#dee8f1;
		}
		</style>
	</head>
	<body>
	<h1>Messages</h1>
	<table>
		<colgroup>
            <col style="width:80px"/>
            <col style="width:120px"/>
            <col style="width:160px"/>
            <col style="width:180px"/>
        </colgroup>
		<tr>
			<th>Type</th>
			<th>Number</th>
			<th>Name</th>
			<th>Date</th>
			<th>Message</th>
		</tr>
		<xsl:for-each select="messages/*">
		<tr>
			<td>
				<xsl:if test="@msg_box = 1">
				Received
				</xsl:if>
				<xsl:if test="@msg_box = 2">
				Sent
				</xsl:if>
				<xsl:if test="@msg_box = 3">
				Draft
				</xsl:if>
			</td>
			<td><xsl:value-of select="@from"/></td>
			<td><xsl:value-of select="@contact_name"/></td>
			<td><xsl:value-of select="@readable_date"/></td>
			<td>
				<xsl:value-of select="@body"/>
				<xsl:for-each select="attachments/attachment">
					<a>
						<xsl:attribute name="href">
							<xsl:value-of select="@src"/>
						</xsl:attribute>
						<xsl:value-of select="@src"/>
					</a>
				</xsl:for-each>
				<xsl:for-each select="parts/part">
					<xsl:choose>
						<xsl:when test="starts-with(@content_type,'text/')" >
							<xsl:value-of select="@text"/><br/>
						</xsl:when>
						<xsl:when test="starts-with(@content_type,'image/')" >
							<img height="300">
								<xsl:attribute name="src">
									<xsl:value-of select="concat(concat('data:',@content_type), concat(';base64,',@data))"/>
								</xsl:attribute>
							</img><br/>
						</xsl:when>
						<xsl:otherwise>
							<i>Preview of <xsl:value-of select="@ct"/> not supported.</i><br/>
						</xsl:otherwise>
					</xsl:choose>
				</xsl:for-each>
			</td>
		</tr>
		</xsl:for-each>
	</table>
	</body>
</html>
</xsl:template>
</xsl:stylesheet>
