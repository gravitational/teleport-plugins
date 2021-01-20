const ejs = require("ejs");
const express = require("express");
const jwksClient = require("jwks-rsa");
const jwt = require("jsonwebtoken");
const process = require("process");
const url = require("url");

const dotenv = require('dotenv');
dotenv.config();

const listenPort = parseInt(process.env.PORT || 8080);
const isInsecure =
  process.env.TELEPORT_INSECURE === "true"
    ? true
    : process.env.TELEPORT_INSECURE === "TRUE"
    ? true
    : process.env.TELEPORT_INSECURE === "t"
    ? true
    : process.env.TELEPORT_INSECURE === "1"
    ? true
    : false;

let proxyAddr = process.env.TELEPORT_PROXY || "https://example.teleport.sh:443";

if (!proxyAddr.match(/(http|https):\/\//)) {
  proxyAddr = "https://" + proxyAddr;
}

const app = express();
const jwks = jwksClient({
  strictSsl: !isInsecure,
  cache: true, // Public key must be memoized.
  jwksUri: new url.URL("/.well-known/jwks.json", proxyAddr).toString(),
});

function getKey(header, callback) {
  // For now, Teleport's `header.kid` is undefined but lets pass it for generality.
  jwks.getSigningKey(header.kid, function (err, key) {
    if (err) {
      callback(err);
      return;
    }
    callback(null, key.getPublicKey());
  });
}

// Simple middleware that verifies JWT token.
app.use(function (req, res, next) {
  const token = req.headers["teleport-jwt-assertion"];
  if (!token) {
    res.status(403).send("Access denied");
    return;
  }
  jwt.verify(token, getKey, function (err, decoded) {
    if (err) {
      res.status(403).send("Access denied");
      return;
    }
    req.teleportJWT = decoded;
    next();
  });
});

// Main endpoint.
// Prints users Teleport Username and all roles for that user.
app.get("/", function (req, res) {
  const { username, roles } = req.teleportJWT;
  res.send(
    ejs.render(
      `
    <p>Hello <b><%= username %></b>!</p>
   <p>You are now logged in and have these roles.
   <ul>
     <% roles.forEach(role => { %>
       <li><%= role %></li>
     <% }); %>
   </ul>
`,
      { username: username, roles: roles }
    )
  );
});

app.listen(listenPort, function () {
  console.log(`Listening on ${listenPort}...`);
});